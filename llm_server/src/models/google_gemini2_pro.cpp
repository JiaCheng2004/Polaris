#include "models/google_gemini2_pro.hpp"
#include "utils/logger.hpp"

namespace models {

ModelResult GoogleGemini2Pro::uploadAndQuery(const nlohmann::json& input,
                                        const std::vector<crow::multipart::part>& fileParts)
{
    utils::Logger::info("GoogleGemini2Pro: fileParts.size() = " + std::to_string(fileParts.size()));

    ModelResult result;
    result.success   = true;
    result.modelUsed = "google-gemini-2.0-pro";
    result.message   = "Hello World";
    result.tokenUsage = 666;

    return result;
}

} // namespace models
