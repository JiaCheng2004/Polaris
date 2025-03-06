#include "utils/http_utils.hpp"
#include "utils/logger.hpp"

#include <curl/curl.h>
#include <stdexcept>

namespace
{
    // Helper callback to collect response data
    static size_t writeCallback(void *contents, size_t size, size_t nmemb, void *userp)
    {
        ((std::string*)userp)->append((char*)contents, size * nmemb);
        return size * nmemb;
    }
}

namespace http_utils
{

HttpResponse performMultipartPost(const std::string &url,
                                  const std::vector<std::string> &headers,
                                  const std::map<std::string, std::string> &formFields,
                                  const std::string &fileFieldName,
                                  const std::string &filePath)
{
    HttpResponse httpResponse;
    CURL *curl = curl_easy_init();
    if (!curl) {
        throw std::runtime_error("Failed to initialize curl.");
    }

    // Convert headers from std::vector<std::string> to curl_slist
    struct curl_slist *curlHeaders = nullptr;
    for (auto &h : headers)
    {
        curlHeaders = curl_slist_append(curlHeaders, h.c_str());
    }

    // Build the multipart form
    curl_mime *mime = curl_mime_init(curl);
    curl_mimepart *part = nullptr;

    // Add text fields
    for (const auto &kv : formFields)
    {
        part = curl_mime_addpart(mime);
        curl_mime_name(part, kv.first.c_str());
        curl_mime_data(part, kv.second.c_str(), CURL_ZERO_TERMINATED);
    }

    // Add the file
    part = curl_mime_addpart(mime);
    curl_mime_name(part, fileFieldName.c_str()); 
    curl_mime_filedata(part, filePath.c_str());

    // Set options
    curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, curlHeaders);
    curl_easy_setopt(curl, CURLOPT_MIMEPOST, mime);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);
    
    // Collect response in httpResponse.body
    std::string responseString;
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseString);

    // Perform
    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK)
    {
        utils::Logger::error(std::string("[performMultipartPost] curl error: ") + curl_easy_strerror(res));
    }

    // Get status code
    long httpCode = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &httpCode);
    httpResponse.statusCode = httpCode;
    httpResponse.body = responseString;

    // Cleanup
    curl_mime_free(mime);
    curl_slist_free_all(curlHeaders);
    curl_easy_cleanup(curl);

    return httpResponse;
}

HttpResponse performJsonPost(const std::string &url,
                             const std::vector<std::string> &headers,
                             const nlohmann::json &jsonBody)
{
    HttpResponse httpResponse;
    CURL *curl = curl_easy_init();
    if (!curl) {
        throw std::runtime_error("Failed to initialize curl.");
    }

    // Convert headers
    struct curl_slist *curlHeaders = nullptr;
    for (auto &h : headers)
    {
        curlHeaders = curl_slist_append(curlHeaders, h.c_str());
    }

    // Body as string
    std::string bodyStr = jsonBody.dump();

    curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, curlHeaders);
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, bodyStr.c_str());
    curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, bodyStr.size());
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);

    std::string responseString;
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseString);

    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK)
    {
        utils::Logger::error(std::string("[performJsonPost] curl error: ") + curl_easy_strerror(res));
    }

    long httpCode = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &httpCode);
    httpResponse.statusCode = httpCode;
    httpResponse.body = responseString;

    curl_slist_free_all(curlHeaders);
    curl_easy_cleanup(curl);

    return httpResponse;
}

HttpResponse performGet(const std::string &url,
                        const std::vector<std::string> &headers)
{
    HttpResponse httpResponse;
    CURL *curl = curl_easy_init();
    if (!curl) {
        throw std::runtime_error("Failed to initialize curl.");
    }

    // Convert headers
    struct curl_slist *curlHeaders = nullptr;
    for (auto &h : headers)
    {
        curlHeaders = curl_slist_append(curlHeaders, h.c_str());
    }

    curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, curlHeaders);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, writeCallback);

    std::string responseString;
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseString);

    CURLcode res = curl_easy_perform(curl);
    if (res != CURLE_OK)
    {
        utils::Logger::error(std::string("[performGet] curl error: ") + curl_easy_strerror(res));
    }

    long httpCode = 0;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &httpCode);
    httpResponse.statusCode = httpCode;
    httpResponse.body = responseString;

    curl_slist_free_all(curlHeaders);
    curl_easy_cleanup(curl);

    return httpResponse;
}

} // namespace http_utils
