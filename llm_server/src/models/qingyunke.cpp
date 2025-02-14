#include "models/qingyunke.hpp"
#include <curl/curl.h>
#include <stdexcept>
#include <sstream>

namespace {
    // Callback to collect response data from CURL
    size_t writeCallback(void* contents, size_t size, size_t nmemb, void* userp) {
        ((std::string*)userp)->append((char*)contents, size * nmemb);
        return size * nmemb;
    }
}

namespace models {

nlohmann::json Qingyunke::query(const std::string& userMessage) {
    nlohmann::json responseJson;
    responseJson["model_used"]  = "qingyunke";
    responseJson["message"]     = "";
    responseJson["token_usage"] = 0;  // For demonstration, set to 0
    responseJson["error_code"]  = 200;
    responseJson["details"]     = "OK";

    // Construct URL: http://api.qingyunke.com/api.php?key=free&appid=0&msg=<encoded user message>
    std::ostringstream urlStream;
    urlStream << "http://api.qingyunke.com/api.php"
              << "?key=free"
              << "&appid=0"
              << "&msg=" << userMessage; // Note: for production, you should URL-encode userMessage!

    CURL* curl = curl_easy_init();
    if (!curl) {
        responseJson["error_code"] = 500;
        responseJson["details"] = "Failed to initialize CURL";
        return responseJson;
    }

    std::string readBuffer;
    curl_easy_setopt(curl, CURLOPT_URL, urlStream.str().c_str());
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &readBuffer);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10L);

    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        responseJson["error_code"] = 500;
        responseJson["details"] = "CURL request failed: " + std::string(curl_easy_strerror(res));
        curl_easy_cleanup(curl);
        return responseJson;
    }

    // Parse the JSON from the response
    // Expected example: {"result":0,"content":"你好，我就开心了"}
    try {
        auto apiResponse = nlohmann::json::parse(readBuffer);
        // Assume "content" field holds the returned text
        if (apiResponse.contains("content")) {
            responseJson["message"] = apiResponse["content"].get<std::string>();
        } else {
            responseJson["error_code"] = 500;
            responseJson["details"] = "No 'content' field in API response";
        }
    } catch (const std::exception& e) {
        responseJson["error_code"] = 500;
        responseJson["details"] = std::string("JSON parse error: ") + e.what();
    }

    curl_easy_cleanup(curl);
    return responseJson;
}

} // namespace models
