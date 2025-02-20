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
    std::string modelUsed;          ///< The model identifier (e.g., "openai-gpt-4")
    std::string message;            ///< Main output or message returned by the model
    int tokenUsage = 0;             ///< Number of tokens used
    int errorCode = 200;            ///< HTTP-like error code (200 for OK)
    std::string errorMessage;       ///< Additional error message if not successful
    std::vector<std::string> fileIds; ///< Optional list of file IDs returned by the model
};

} // namespace models

#endif // MODEL_RESULT_HPP
