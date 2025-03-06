#include "utils/openai/utils.hpp"
#include "utils/http_utils.hpp"
#include "utils/logger.hpp"

#include <unordered_set>
#include <filesystem>
#include <fstream>
#include <algorithm>
#include <cctype>
#include <string>
#include <thread>
#include <chrono>

namespace fs = std::filesystem;

namespace openai_utils
{

std::string getFileExtension(const std::string &filename)
{
    fs::path p(filename);
    std::string ext = p.extension().string();
    if (!ext.empty() && ext.front() == '.')
    {
        ext.erase(0, 1);
    }
    // Convert to lowercase
    std::transform(ext.begin(), ext.end(), ext.begin(),
                   [](unsigned char c){ return std::tolower(c); });
    return ext;
}

// 2) isExtensionSupported (overload for set)
bool isExtensionSupported(const std::string &ext,
    const std::unordered_set<std::string> &allowed)
{
return allowed.count(ext) > 0;
}

// 2b) Alternatively for vector
bool isExtensionSupported(const std::string &ext,
    const std::vector<std::string> &allowed)
{
for (auto &a : allowed)
{
if (a == ext) return true;
}
return false;
}

std::string uploadFile(const std::string &localFilePath, const std::string &openAIKey)
{
    // Prepare headers
    std::vector<std::string> headers = {
        "Authorization: Bearer " + openAIKey
        // Content-Type is set automatically by libcurl for multipart
    };

    // The form field "purpose" must be set to "assistants" (according to your instructions)
    std::map<std::string, std::string> formFields;
    formFields["purpose"] = "assistants";

    // Actually do the real POST to the real endpoint
    auto resp = http_utils::performMultipartPost(
        "https://api.openai.com/v1/files",
        headers,
        formFields,
        "file",
        localFilePath
    );

    if (resp.statusCode < 200 || resp.statusCode >= 300)
    {
        utils::Logger::error("[uploadFile] Upload failed. Status: "
            + std::to_string(resp.statusCode)
            + " Body: " + resp.body);
        return "";
    }

    // Parse JSON for "id"
    try
    {
        auto jResp = nlohmann::json::parse(resp.body);
        if (jResp.contains("id"))
        {
            std::string fileId = jResp["id"].get<std::string>();
            utils::Logger::info("[uploadFile] Successfully uploaded file. File ID: " + fileId);
            return fileId;
        }
    }
    catch (std::exception &ex)
    {
        utils::Logger::error(std::string("[uploadFile] JSON parse error: ") + ex.what());
    }
    return "";
}

std::string createThread(const nlohmann::json &messages, const std::string &openAIKey)
{
    // According to your instructions, we do:
    //   POST https://api.openai.com/v1/threads
    //   Body: { "messages": [ ... ] }
    // with "OpenAI-Beta: assistants=v2"
    std::vector<std::string> headers = {
        "Content-Type: application/json",
        "Authorization: Bearer " + openAIKey,
        "OpenAI-Beta: assistants=v2"
    };

    nlohmann::json body;
    body["messages"] = messages;

    auto resp = http_utils::performJsonPost("https://api.openai.com/v1/threads", headers, body);

    if (resp.statusCode < 200 || resp.statusCode >= 300)
    {
        utils::Logger::error("[createThread] Creation failed. Status: "
                             + std::to_string(resp.statusCode)
                             + " Body: " + resp.body);
        return "";
    }

    try
    {
        auto jResp = nlohmann::json::parse(resp.body);
        if (jResp.contains("id"))
        {
            std::string threadId = jResp["id"].get<std::string>();
            utils::Logger::info("[createThread] Created thread. ID: " + threadId);
            return threadId;
        }
        else
        {
            utils::Logger::error("[createThread] 'id' not found in response: " + resp.body);
        }
    }
    catch (std::exception &ex)
    {
        utils::Logger::error(std::string("[createThread] JSON parse error: ") + ex.what());
    }
    return "";
}

std::string createRun(const std::string &threadId,
                      const std::string &openAIKey,
                      const std::string &assistantId)
{
    std::vector<std::string> headers = {
        "Content-Type: application/json",
        "Authorization: Bearer " + openAIKey,
        "OpenAI-Beta: assistants=v2"
    };

    nlohmann::json body;
    body["assistant_id"] = assistantId;

    std::string url = "https://api.openai.com/v1/threads/" + threadId + "/runs";
    auto resp = http_utils::performJsonPost(url, headers, body);

    if (resp.statusCode < 200 || resp.statusCode >= 300)
    {
        utils::Logger::error("[createRun] Run creation failed. Status: "
                             + std::to_string(resp.statusCode)
                             + " Body: " + resp.body);
        return "";
    }

    try
    {
        auto jResp = nlohmann::json::parse(resp.body);
        if (jResp.contains("id"))
        {
            std::string runId = jResp["id"].get<std::string>();
            utils::Logger::info("[createRun] Created run. ID: " + runId);
            return runId;
        }
        else
        {
            utils::Logger::error("[createRun] 'id' not found in response: " + resp.body);
        }
    }
    catch (std::exception &ex)
    {
        utils::Logger::error(std::string("[createRun] JSON parse error: ") + ex.what());
    }
    return "";
}

bool waitForRunCompletion(const std::string &threadId,
                          const std::string &runId,
                          const std::string &openAIKey,
                          int &outUsage,
                          int maxRetries)
{
    std::vector<std::string> headers = {
        "Authorization: Bearer " + openAIKey,
        "OpenAI-Beta: assistants=v2"
    };

    std::string url = "https://api.openai.com/v1/threads/" + threadId + "/runs/" + runId;

    for (int i = 0; i < maxRetries; i++)
    {
        auto resp = http_utils::performGet(url, headers);
        if (resp.statusCode < 200 || resp.statusCode >= 300)
        {
            utils::Logger::error("[waitForRunCompletion] Error retrieving run status. Code: "
                                 + std::to_string(resp.statusCode)
                                 + " Body: " + resp.body);
            return false; // or keep polling?
        }

        try
        {
            auto jResp = nlohmann::json::parse(resp.body);
            if (jResp.contains("status"))
            {
                std::string status = jResp["status"].get<std::string>();
                utils::Logger::info("[waitForRunCompletion] Current run status: " + status);

                if (status == "completed")
                {
                    // Possibly gather usage
                    if (jResp.contains("usage") && jResp["usage"].contains("total_tokens"))
                    {
                        outUsage = jResp["usage"]["total_tokens"].get<int>();
                    }
                    return true;
                }
                else if (status == "failed" || status == "cancelled" || status == "expired")
                {
                    utils::Logger::error("[waitForRunCompletion] Run ended with status: " + status);
                    return false;
                }
            }
        }
        catch (std::exception &ex)
        {
            utils::Logger::error(std::string("[waitForRunCompletion] JSON parse error: ") + ex.what());
            return false;
        }

        // Sleep 1s
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }
    utils::Logger::error("[waitForRunCompletion] Timed out waiting for run completion.");
    return false;
}

std::string getLastMessageId(const std::string &threadId, const std::string &openAIKey)
{
    std::vector<std::string> headers = {
        "Content-Type: application/json",
        "Authorization: Bearer " + openAIKey,
        "OpenAI-Beta: assistants=v2"
    };

    std::string url = "https://api.openai.com/v1/threads/" + threadId + "/messages";
    auto resp = http_utils::performGet(url, headers);

    if (resp.statusCode < 200 || resp.statusCode >= 300)
    {
        utils::Logger::error("[getLastMessageId] Failed. Code: "
                             + std::to_string(resp.statusCode)
                             + " Body: " + resp.body);
        return "";
    }

    try
    {
        auto jResp = nlohmann::json::parse(resp.body);
        if (jResp.contains("first_id"))
        {
            return jResp["first_id"].get<std::string>();
        }
        else
        {
            utils::Logger::error("[getLastMessageId] 'first_id' not found in response: " + resp.body);
        }
    }
    catch (std::exception &ex)
    {
        utils::Logger::error(std::string("[getLastMessageId] JSON parse error: ") + ex.what());
    }
    return "";
}

nlohmann::json getMessageById(const std::string &threadId,
                              const std::string &messageId,
                              const std::string &openAIKey)
{
    nlohmann::json emptyResult;
    std::vector<std::string> headers = {
        "Content-Type: application/json",
        "Authorization: Bearer " + openAIKey,
        "OpenAI-Beta: assistants=v2"
    };

    std::string url = "https://api.openai.com/v1/threads/" + threadId + "/messages/" + messageId;
    auto resp = http_utils::performGet(url, headers);

    if (resp.statusCode < 200 || resp.statusCode >= 300)
    {
        utils::Logger::error("[getMessageById] Failed. Code: "
                             + std::to_string(resp.statusCode)
                             + " Body: " + resp.body);
        return emptyResult;
    }

    try
    {
        auto jResp = nlohmann::json::parse(resp.body);
        return jResp;
    }
    catch (std::exception &ex)
    {
        utils::Logger::error(std::string("[getMessageById] JSON parse error: ") + ex.what());
    }
    return emptyResult;
}

} // namespace openai_utils
