#include "models/openai_o1.hpp"
#include "utils/logger.hpp"

namespace models {

ModelResult OpenAIo1::uploadAndQuery(const nlohmann::json& input,
                                        const std::vector<crow::multipart::part>& fileParts)
{
    utils::Logger::info("OpenAIo1: fileParts.size() = " + std::to_string(fileParts.size()));

    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-o1";
    result.message   = "Hello World";
    result.tokenUsage = 666;

    return result;
}

} // namespace models
