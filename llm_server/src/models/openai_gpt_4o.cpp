#include "models/openai_gpt_4o.hpp"

namespace models {

ModelResult OpenAIGPT4o::uploadAndQuery(const std::string& context,
                                        const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-gpt-4o";
    // Dummy message
    result.message   = "Hello from OpenAI GPT-4o dummy code.";
    result.tokenUsage = 35;
    result.errorCode = 200; // OK
    
    return result;
}

} // namespace models
