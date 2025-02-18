#ifndef OPENAI_O1_HPP
#define OPENAI_O1_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include <vector>

namespace models {

class OpenAIo1 : public IModel {
public:
    ModelResult uploadAndQuery(const nlohmann::json& input,
                               const std::vector<crow::multipart::part>& fileParts) override;
};

} // namespace models

#endif // OPENAI_O1_HPP
