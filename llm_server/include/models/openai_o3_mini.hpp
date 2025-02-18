#ifndef OPENAI_O3_MINI_HPP
#define OPENAI_O3_MINI_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include <vector>

namespace models {

class OpenAIo3mini : public IModel {
public:
    ModelResult uploadAndQuery(const nlohmann::json& input,
                               const std::vector<crow::multipart::part>& fileParts) override;
};

} // namespace models

#endif // OPENAI_O3_MINI_HPP