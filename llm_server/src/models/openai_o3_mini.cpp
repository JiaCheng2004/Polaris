#include "models/openai_o3_mini.hpp"
#include <stdexcept>
#include <sstream>
#include <fstream>
#include <curl/curl.h>
#include <nlohmann/json.hpp>

// If you have a global config in another .cpp:
extern nlohmann::json g_config;

namespace
{
/**
 * Helper: cURL callback to write response into a std::stringstream.
 */
size_t writeCallback(char* ptr, size_t size, size_t nmemb, void* userdata)
{
    std::stringstream* stream = static_cast<std::stringstream*>(userdata);
    size_t totalBytes = size * nmemb;
    stream->write(ptr, static_cast<std::streamsize>(totalBytes));
    return totalBytes;
}

/**
 * Extract filename from Crow's part.headers["Content-Disposition"].
 */
std::string getFilename(const crow::multipart::part& part)
{
    // By default, crow::multipart::part::headers is a map of 
    //  std::string -> crow::multipart::header { name, value }
    auto it = part.headers.find("Content-Disposition");
    if (it != part.headers.end()) {
        // The actual header text is in `it->second.value`, e.g. 
        //   "form-data; name=\"file\"; filename=\"myfile.txt\""
        const std::string& disp = it->second.value;
        const std::string marker = "filename=\"";
        auto pos = disp.find(marker);
        if (pos != std::string::npos) {
            pos += marker.size();
            auto endPos = disp.find('"', pos);
            if (endPos != std::string::npos) {
                return disp.substr(pos, endPos - pos);
            }
        }
    }
    // Fallback if not found
    return "uploaded_file";
}

/**
 * Helper: calls OpenAI ChatCompletion with the given messages + file IDs.
 */
nlohmann::json callOpenAIChatCompletion(
    const std::string& apiKey,
    const std::vector<models::ChatMessage>& messages,
    const std::vector<std::string>& fileIds,
    const std::string& reasoningEffort
)
{
    // 1) Construct request JSON
    nlohmann::json reqBody;
    reqBody["model"] = "o3-mini"; // or "gpt-3.5-turbo", etc.

    nlohmann::json msgs = nlohmann::json::array();
    for (auto& m : messages) {
        nlohmann::json msgObj;
        msgObj["role"]    = m.role;
        msgObj["content"] = m.content;
        msgs.push_back(msgObj);
    }
    reqBody["messages"] = msgs;

    if (!fileIds.empty()) {
        reqBody["file_ids"] = fileIds;  // if your custom logic needs these
    }

    // Possibly set temperature by reasoningEffort
    reqBody["temperature"] = (reasoningEffort == "low") ? 1.0 : 0.7;

    // 2) Setup cURL
    CURL* curl = curl_easy_init();
    if (!curl) {
        throw std::runtime_error("Failed to init cURL handle");
    }

    // 3) Headers
    struct curl_slist* headers = nullptr;
    std::string authHeader = "Authorization: Bearer " + apiKey;
    headers = curl_slist_append(headers, authHeader.c_str());
    headers = curl_slist_append(headers, "Content-Type: application/json");

    // 4) Convert reqBody to string
    std::string bodyStr = reqBody.dump();

    // 5) cURL config
    curl_easy_setopt(curl, CURLOPT_URL, "https://api.openai.com/v1/chat/completions");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, bodyStr.c_str());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, bodyStr.size());

    // 6) Capture response
    std::stringstream responseStream;
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseStream);

    // 7) Perform
    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        std::string err = curl_easy_strerror(res);
        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        throw std::runtime_error("cURL error: " + err);
    }

    // 8) Check HTTP code
    long httpCode = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &httpCode);

    // Cleanup
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    // Parse JSON
    nlohmann::json respJson;
    try {
        respJson = nlohmann::json::parse(responseStream.str());
    } catch (...) {
        respJson = {{"error", {{"message", "Failed to parse JSON from OpenAI"}}}};
    }

    if (httpCode != 200) {
        if (!respJson.contains("error")) {
            respJson["error"] = {
                {"message", "OpenAI returned HTTP " + std::to_string(httpCode)},
                {"type", "unknown_error"}
            };
        }
    }

    return respJson;
}

} // end anonymous namespace

//========================================================
//  Now define everything inside namespace models
//========================================================
namespace models
{

/**
 * Implementation of OpenAIo3mini::uploadAndQuery
 * (Signature must match exactly what's in the .hpp)
 */
ModelResult OpenAIo3mini::uploadAndQuery(
    const std::vector<ChatMessage>& messages,
    const std::vector<crow::multipart::part>& fileParts,
    const std::string& reasoningEffort
)
{
    ModelResult result;  // from utils/model_common.hpp
    result.success      = true;
    result.errorCode    = 200;
    result.errorMessage = "";
    result.modelUsed    = "openai-o3-mini";
    result.message      = "";
    result.tokenUsage   = 0;
    result.fileIds.clear();

    // 1) Retrieve OpenAI API key from global g_config
    std::string apiKey;
    try {
        apiKey = g_config.at("openai").at("apikey").get<std::string>();
    } catch (...) {
        result.success      = false;
        result.errorCode    = 500;
        result.errorMessage = "Missing 'openai.apikey' in config.json";
        return result;
    }

    // 2) If there are files, upload them
    for (const auto& part : fileParts) {
        bool fileUploaded = false;
        UploadResult uploadRes; // from this same .hpp

        // Derive a filename from Content-Disposition:
        std::string actualFilename = getFilename(part);
        std::string tempFilename   = "/tmp/" + actualFilename;

        // Write part.body to temp file
        {
            std::ofstream ofs(tempFilename, std::ios::binary);
            ofs.write(part.body.data(), static_cast<std::streamsize>(part.body.size()));
        }

        // Attempt up to 10 times
        for (int attempt = 1; attempt <= 10; ++attempt) {
            uploadRes = uploadFileOpenAI(tempFilename, actualFilename, apiKey, "assistants");
            if (uploadRes.success) {
                fileUploaded = true;
                break;
            }
        }

        if (!fileUploaded) {
            result.success      = false;
            result.errorCode    = 500;
            result.errorMessage = "Failed to upload file '" + actualFilename
                                  + "' after 10 attempts: " + uploadRes.errorReason;
            return result;
        }
        // Record the file ID
        result.fileIds.push_back(uploadRes.fileId);
    }

    // 3) Call ChatCompletions endpoint
    nlohmann::json respJson;
    try {
        respJson = callOpenAIChatCompletion(apiKey, messages, result.fileIds, reasoningEffort);
    } catch (const std::exception& e) {
        result.success      = false;
        result.errorCode    = 500;
        result.errorMessage = "Exception in callOpenAIChatCompletion: " + std::string(e.what());
        return result;
    }

    // 4) Check if there's an "error"
    if (respJson.contains("error")) {
        auto errObj = respJson["error"];
        std::string msg  = errObj.value("message", "Unknown error");
        std::string code = errObj.value("code", "unknown_code");

        result.success      = false;
        result.errorCode    = (code == "invalid_api_key") ? 401 : 400;
        result.errorMessage = "OpenAI error: " + msg + " (code: " + code + ")";
        return result;
    }

    // 5) Extract text from "choices[0].message.content" if available
    if (respJson.contains("choices") && respJson["choices"].is_array() && !respJson["choices"].empty()) {
        auto& firstChoice = respJson["choices"][0];
        if (firstChoice.contains("message")) {
            result.message = firstChoice["message"].value("content", "");
        }
    }

    // 6) If usage is present, get total_tokens
    if (respJson.contains("usage")) {
        auto usage = respJson["usage"];
        if (usage.contains("total_tokens")) {
            result.tokenUsage = usage.value("total_tokens", 0);
        }
    }

    return result;
}

/**
 * Implementation of OpenAIo3mini::uploadFileOpenAI
 * Matches the private method signature in the .hpp
 */
UploadResult OpenAIo3mini::uploadFileOpenAI(
    const std::string& filePath,
    const std::string& filename,
    const std::string& apiKey,
    const std::string& purpose)
{
    UploadResult result;
    result.success     = false;
    result.fileId      = "";
    result.errorReason = "";

    static const std::string url = "https://api.openai.com/v1/files";

    // Initialize cURL
    CURL* curl = curl_easy_init();
    if (!curl) {
        result.errorReason = "Failed to init curl";
        return result;
    }

    // Build headers
    struct curl_slist* headers = nullptr;
    std::string authHeader = "Authorization: Bearer " + apiKey;
    headers = curl_slist_append(headers, authHeader.c_str());
    // "Expect:" avoids a 100-continue
    headers = curl_slist_append(headers, "Expect:");

    // Build multipart form
    curl_mime* mime = curl_mime_init(curl);

    // 1) Add "purpose" field
    curl_mimepart* partPurpose = curl_mime_addpart(mime);
    curl_mime_name(partPurpose, "purpose");
    curl_mime_data(partPurpose, purpose.c_str(), CURL_ZERO_TERMINATED);

    // 2) Add file field
    curl_mimepart* filePart = curl_mime_addpart(mime);
    curl_mime_name(filePart, "file");
    curl_mime_filedata(filePart, filePath.c_str()); // read from disk
    curl_mime_filename(filePart, filename.c_str()); // displayed server side

    // cURL options
    curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_MIMEPOST, mime);

    // Capture the response
    std::stringstream responseBuffer;
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION,
        [](char* ptr, size_t size, size_t nmemb, void* userdata) -> size_t {
            std::stringstream* stream = static_cast<std::stringstream*>(userdata);
            size_t totalBytes = size * nmemb;
            stream->write(ptr, static_cast<std::streamsize>(totalBytes));
            return totalBytes;
        }
    );
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseBuffer);

    // Perform the request
    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        result.errorReason = std::string("curl_easy_perform() failed: ") + curl_easy_strerror(res);
    } else {
        // Check HTTP code
        long httpCode = 0;
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &httpCode);

        // Parse JSON
        std::string respStr = responseBuffer.str();
        nlohmann::json respJson;
        try {
            respJson = nlohmann::json::parse(respStr);
        } catch (...) {
            respJson = nlohmann::json::object();
        }

        if (httpCode == 200) {
            // For success, we expect an "id"
            if (respJson.contains("id")) {
                result.success = true;
                result.fileId  = respJson.value("id", "");
            } else {
                result.errorReason = "No 'id' found in response: " + respStr;
            }
        } else {
            // Some error from OpenAI
            if (respJson.contains("error")) {
                auto errObj = respJson["error"];
                std::string msg  = errObj.value("message", "");
                std::string code = errObj.value("code", "");
                result.errorReason = "OpenAI error: " + msg + " (code: " + code + ")";
            } else {
                result.errorReason =
                    "HTTP code: " + std::to_string(httpCode) + " response: " + respStr;
            }
        }
    }

    // Cleanup
    curl_mime_free(mime);
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    return result;
}

} // end namespace models
