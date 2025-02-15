// include/models/model_common.hpp
#ifndef MODEL_COMMON_HPP
#define MODEL_COMMON_HPP

#include <string>
#include <vector>

namespace models {

/**
 * A structure to represent a single chat message, with a role and content.
 */
struct ChatMessage {
    std::string role;    // "system", "user", "assistant", etc.
    std::string content; // text content of the message
};

/**
 * ModelResult is a structure returned by any model after processing
 * a query (including optional file uploads).
 */
struct ModelResult {
    bool success;                ///< Did the query (and file uploads) succeed?
    int  errorCode;              ///< If success=false, an error code (400, 500, etc.)
    std::string errorMessage;    ///< If success=false, reason for failure
    std::string modelUsed;       ///< e.g. "openai-o3-mini" or "gemini-2-pro"
    std::string message;         ///< The main text answer from the model
    int tokenUsage;              ///< If relevant
    // We also store file IDs or any other relevant info
    std::vector<std::string> fileIds;
};

}

#endif // MODEL_COMMON_HPP
