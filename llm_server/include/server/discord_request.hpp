#ifndef REQUEST_HANDLER_HPP
#define REQUEST_HANDLER_HPP

#include "utils/multipart_utils.hpp"
#include <nlohmann/json.hpp>
#include <string>
#include <vector>

namespace server
{
/**
 * @brief Handles language model queries, with optional multipart file uploads.
 *
 * @param input     The JSON input containing model request parameters.
 * @param fileParts A list of file attachments (if any).
 * @return A JSON object containing the model's response or any error information.
 */
nlohmann::json handleDiscordBotLLMQuery(
    const nlohmann::json &input,
    const std::vector<utils::MultipartPart> &fileParts);

} // namespace server

#endif // REQUEST_HANDLER_HPP
