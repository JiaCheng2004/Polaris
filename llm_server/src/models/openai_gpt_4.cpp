#include "models/openai_gpt_4.hpp"

namespace models {

ModelResult OpenAIGPT4::uploadAndQuery(const std::string& context,
                                       const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-gpt-4";
    // Dummy message
    result.message   = "Hello from OpenAI GPT-4 dummy code.";
    result.tokenUsage = 42;
    result.errorCode = 200; // OK
    
    // No file IDs (unless you want to pass back something from fileParts)
    // for (auto& part : fileParts) {
    //     result.fileIds.push_back("dummy_file_id_" + part.name);
    // }

    return result;
}

} // namespace models
