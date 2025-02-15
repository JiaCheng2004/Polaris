#ifndef REQUEST_HANDLER_HPP
#define REQUEST_HANDLER_HPP

#include <nlohmann/json.hpp>
#include <crow/multipart.h> 

namespace server {

/**
 * The function that handles a raw JSON request body (application/json).
 */
nlohmann::json handleLLMQuery(const nlohmann::json& input);

/**
 * The function that handles multipart/form-data (with optional files).
 * Make sure the signature matches what you use in http_server.cpp.
 */
nlohmann::json handleLLMQuery(const nlohmann::json& input,
                              const std::vector<crow::multipart::part>& fileParts);

/**
 * Alternative approach:
 *   or a distinct function if you want to accept the entire crow::multipart::message
 *   and parse inside the handler.
 */
nlohmann::json handleLLMQueryMultipart(const crow::multipart::message& multipartReq);

} // namespace server

#endif // REQUEST_HANDLER_HPP
