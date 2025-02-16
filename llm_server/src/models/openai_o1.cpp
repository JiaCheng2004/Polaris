#include "models/openai_o1.hpp"

namespace models {

ModelResult OpenAIo1::uploadAndQuery(const std::string& context,
                                     const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-o1";
    // Dummy message
    result.message   = "Hello from OpenAI o1 dummy code.";
    result.tokenUsage = 10;
    result.errorCode = 200; // OK
    
    return result;
}

} // namespace models
