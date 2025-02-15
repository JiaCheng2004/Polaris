#include "models/openai_o3_mini.hpp"
#include "utils/logger.hpp"

#include <curl/curl.h>
#include <nlohmann/json.hpp>
#include <stdexcept>
#include <sstream>

extern nlohmann::json g_config;

/**
 * Helper callback to store response data from cURL
 */
namespace {
    size_t writeCallback(void* contents, size_t size, size_t nmemb, void* userp) {
        ((std::string*)userp)->append((char*)contents, size * nmemb);
        return size * nmemb;
    }
}

namespace models {

nlohmann::json OpenAIo3mini::query(const std::vector<ChatMessage>& messages,
                                   const std::string& reasoningEffort)
{
    using json = nlohmann::json;

    // Standard JSON response structure
    json response;
    response["model_used"]  = "openai-o3-mini";
    response["message"]     = "";
    response["files"]       = json::array();
    response["token_usage"] = 0;
    response["error_code"]  = 200;
    response["details"]     = "OK";

    // Retrieve the OpenAI API key from config
    std::string apiKey;
    try {
        apiKey = g_config.at("openai").at("apikey").get<std::string>();
    } catch (...) {
        response["error_code"] = 500;
        response["details"]    = "Missing 'openai.apikey' in config.json";
        return response;
    }

    // Build the request body to match the o3-mini format
    // Example from user snippet:
    // {
    //   "model": "o3-mini",
    //   "reasoning_effort": "high",
    //   "messages": [ {role: "system", content: "..."}, ... ]
    // }
    json requestBody;
    requestBody["model"] = "o3-mini";
    requestBody["reasoning_effort"] = reasoningEffort;

    // Convert our vector of ChatMessage to the "messages" array
    json messagesArray = json::array();
    for (const auto& msg : messages) {
        json m;
        m["role"]    = msg.role;
        m["content"] = msg.content;
        messagesArray.push_back(m);
    }
    requestBody["messages"] = messagesArray;

    // Prepare cURL
    CURL* curl = curl_easy_init();
    if (!curl) {
        response["error_code"] = 500;
        response["details"]    = "Failed to initialize cURL.";
        return response;
    }

    std::string readBuffer;
    struct curl_slist* headers = nullptr;

    // Set cURL options
    curl_easy_setopt(curl, CURLOPT_URL, "https://api.openai.com/v1/chat/completions");
    curl_easy_setopt(curl, CURLOPT_POST, 1L);

    // Headers
    std::string authHeader = "Authorization: Bearer " + apiKey;
    headers = curl_slist_append(headers, authHeader.c_str());
    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);

    // Convert requestBody to string and attach as POSTFIELDS
    const auto requestBodyString = requestBody.dump();
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, requestBodyString.c_str());

    // Set callback to receive response data
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &readBuffer);

    // Perform the request
    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        response["error_code"] = 500;
        response["details"]    = std::string("cURL error: ") + curl_easy_strerror(res);
        curl_slist_free_all(headers);
        curl_easy_cleanup(curl);
        return response;
    }

    // Parse the JSON response
    // Example response (simplified):
    // {
    //   "id": "chatcmpl-...",
    //   "choices": [
    //     {
    //       "message": {
    //         "role": "assistant",
    //         "content": "..."
    //       }
    //     }
    //   ],
    //   "usage": {
    //     "prompt_tokens": 27,
    //     "completion_tokens": 11253,
    //     "total_tokens": 11280
    //   }
    // }
    try {
        auto result = json::parse(readBuffer);

        // Extract the response message
        if (result.contains("choices") && !result["choices"].empty()) {
            auto& firstChoice = result["choices"][0];
            if (firstChoice.contains("message")) {
                response["message"] = firstChoice["message"].value("content", "");
            } else {
                response["error_code"] = 500;
                response["details"]    = "Missing 'message' in choices.";
            }
        } else {
            response["error_code"] = 500;
            response["details"]    = "No 'choices' in OpenAI o3-mini response.";
        }

        // Extract token usage if available
        if (result.contains("usage") && result["usage"].contains("total_tokens")) {
            response["token_usage"] = result["usage"]["total_tokens"].get<int>();
        }

    } catch (const std::exception& e) {
        response["error_code"] = 500;
        response["details"]    = std::string("JSON parse error: ") + e.what();
    }

    // Cleanup
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    return response;
}

} // namespace models
