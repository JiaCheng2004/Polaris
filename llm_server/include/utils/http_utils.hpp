#ifndef HTTP_UTILS_HPP
#define HTTP_UTILS_HPP

#include <string>
#include <vector>
#include <map>
#include <nlohmann/json.hpp>

/**
 * @brief Holds a response from an HTTP request
 */
struct HttpResponse
{
    long statusCode = 0;                          ///< HTTP status code (e.g., 200, 404, 500)
    std::string body;                             ///< Raw response body
    std::map<std::string, std::string> headers;   ///< Response headers, if needed
};

namespace http_utils
{
    /**
     * @brief Perform a multipart/form-data POST request using libcurl.
     * @param url The endpoint URL.
     * @param headers Vector of HTTP header strings (e.g., Authorization, Content-Type).
     * @param formFields A map of text fields for the form. (field name -> value)
     * @param fileFieldName Name of the file field (e.g. "file").
     * @param filePath Local path to the file to upload.
     * @return HttpResponse with .statusCode, .body, etc.
     */
    HttpResponse performMultipartPost(const std::string &url,
                                      const std::vector<std::string> &headers,
                                      const std::map<std::string, std::string> &formFields,
                                      const std::string &fileFieldName,
                                      const std::string &filePath);

    /**
     * @brief Perform a JSON POST request with libcurl (Content-Type: application/json).
     * @param url Endpoint URL.
     * @param headers Vector of HTTP header strings.
     * @param jsonBody The JSON body to POST.
     * @return HttpResponse
     */
    HttpResponse performJsonPost(const std::string &url,
                                 const std::vector<std::string> &headers,
                                 const nlohmann::json &jsonBody);

    /**
     * @brief Perform an HTTP GET request using libcurl.
     * @param url Endpoint URL.
     * @param headers Vector of HTTP header strings.
     * @return HttpResponse
     */
    HttpResponse performGet(const std::string &url,
                            const std::vector<std::string> &headers);
} // namespace http_utils

#endif // HTTP_UTILS_HPP
