#include "models/openai_gpt_4o.hpp"
#include "utils/logger.hpp"

namespace models {

ModelResult OpenAIGPT4o::uploadAndQuery(const nlohmann::json& input,
                                        const std::vector<crow::multipart::part>& fileParts)
{
    utils::Logger::info("OpenAIGPT4o: fileParts.size() = " + std::to_string(fileParts.size()));

    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-gpt-4o";
    result.message   = "Hello World";
    result.tokenUsage = 666;

    return result;
}

} // namespace models
