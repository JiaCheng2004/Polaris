#include "models/openai_o3_mini.hpp"
#include "utils/logger.hpp"

namespace models {

ModelResult OpenAIo3mini::uploadAndQuery(const nlohmann::json& input,
                                        const std::vector<crow::multipart::part>& fileParts)
{
    utils::Logger::info("OpenAIo3mini: fileParts.size() = " + std::to_string(fileParts.size()));

    ModelResult result;
    result.success   = true;
    result.modelUsed = "openai-o3-mini";
    result.message   = "Hello World";
    result.tokenUsage = 666;

    return result;
}

} // namespace models
