#ifndef MODEL_RESULT_HPP
#define MODEL_RESULT_HPP

#include <string>
#include <vector>

namespace models
{
/**
 * @brief Struct representing the result of a model operation.
 *
 * Includes success/failure status, error codes and messages, model identifiers,
 * token usage details, and optionally any file IDs.
 */
struct ModelResult
{
    bool success = true;            ///< Whether the model call succeeded
    std::string model_used;          ///< The model identifier (e.g., "gpt4o")
    std::string result;            ///< Main output or message returned by the model
    int token_usage = 0;             ///< Number of tokens used
    int code = 200;            ///< HTTP-like error code (200 for OK)
    std::string message;       ///< Additional error message if not successful
    std::vector<std::string> file_ids; ///< Optional list of file IDs returned by the model
};

} // namespace models

#endif // MODEL_RESULT_HPP
