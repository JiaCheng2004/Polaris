#ifndef IMODEL_HPP
#define IMODEL_HPP

#pragma once

#include "utils/model_result.hpp"
#include "utils/multipart_utils.hpp"
#include <nlohmann/json.hpp>
#include <vector>

namespace models
{
/**
 * @brief Common interface for all LLM models.
 */
class IModel
{
public:
    virtual ~IModel() = default;

    /**
     * @brief Perform a model query given the JSON input and file attachments.
     *
     * @param input     The JSON input containing model request details.
     * @param fileParts A list of file attachments (if any).
     * @return A ModelResult describing success/failure and any model output.
     */
    virtual ModelResult uploadAndQuery(
        const nlohmann::json &input,
        const std::vector<utils::MultipartPart> &fileParts) = 0;
};

} // namespace models

#endif // IMODEL_HPP
