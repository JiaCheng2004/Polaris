#ifndef OPENAI_GPT_4_HPP
#define OPENAI_GPT_4_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/model_result.hpp"
#include "utils/multipart_utils.hpp"
#include <nlohmann/json.hpp>
#include <vector>

namespace models
{
/**
 * @brief Concrete implementation of the IModel interface for OpenAI GPT-4.
 */
class OpenAIGPT4 : public IModel
{
public:
    /**
     * @brief Uploads file parts and queries the GPT-4 model with the provided JSON input.
     * @param input     The JSON input containing model request details.
     * @param fileParts A list of file attachments (if any).
     * @return A ModelResult containing success/failure status, error messages, and any returned data.
     */
    ModelResult uploadAndQuery(
        const nlohmann::json &input,
        const std::vector<utils::MultipartPart> &fileParts) override;
};

} // namespace models

#endif // OPENAI_GPT_4_HPP
