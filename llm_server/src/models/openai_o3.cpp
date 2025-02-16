#include "models/openai_o3.hpp"

namespace models {

ModelResult OpenAIo3mini::uploadAndQuery(const std::string& context,
                                         const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-o3-mini";
    // Dummy message
    result.message   = "Hello from OpenAI o3 mini dummy code.";
    result.tokenUsage = 25;
    result.errorCode = 200; // OK
    
    return result;
}

} // namespace models
