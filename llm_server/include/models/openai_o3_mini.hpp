#ifndef OPENAI_O3_MINI_HPP
#define OPENAI_O3_MINI_HPP

#include <nlohmann/json.hpp>
#include <string>
#include <vector>

/**
 * A structure to represent a single chat message, with a role and content.
 */
struct ChatMessage {
    std::string role;    // "system", "user", or "assistant"
    std::string content; // text content of the message
};

namespace models {

/**
 * OpenAIo3mini class for calling the "o3-mini" model with optional "reasoning_effort" parameter.
 * This is distinct from the existing openai.cpp if you prefer separate implementations.
 */
class OpenAIo3mini {
public:
    /**
     * query - Sends a chat request to OpenAI's "o3-mini" endpoint.
     *
     * @param messages   A vector of ChatMessage objects (system/user/assistant).
     * @param reasoningEffort Could be "high", "medium", "low", etc. Some models might not accept it.
     *
     * @return A JSON object with the standardized fields:
     * {
     *   "model_used": "openai-o3-mini",
     *   "message": "...",
     *   "files": [],
     *   "token_usage": 0,
     *   "error_code": 200,
     *   "details": "OK"
     * }
     */
    static nlohmann::json query(const std::vector<ChatMessage>& messages,
                                const std::string& reasoningEffort = "high");
};

} // namespace models

#endif // OPENAI_O3_MINI_HPP
