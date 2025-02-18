#ifndef IMODEL_HPP
#define IMODEL_HPP

#pragma once

#include "utils/model_result.hpp" // So we can use ModelResult
#include <nlohmann/json.hpp>
#include <crow/multipart.h>
#include <vector>

namespace models {

/**
 * Common interface for all LLM models.
 */
class IModel {
public:
    virtual ~IModel() = default;

    /**
     * Given the entire JSON input and (optional) file parts,
     * perform the desired model query and return a ModelResult.
     */
    virtual ModelResult uploadAndQuery(const nlohmann::json& input,
                                       const std::vector<crow::multipart::part>& fileParts) = 0;
};

} // namespace models

#endif // IMODEL_HPP
