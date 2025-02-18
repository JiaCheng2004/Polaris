#ifndef OPENAI_GPT_4_HPP
#define OPENAI_GPT_4_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include <vector>

namespace models {

class OpenAIGPT4 : public IModel {
public:
    ModelResult uploadAndQuery(const nlohmann::json& input,
                               const std::vector<crow::multipart::part>& fileParts) override;
};

} // namespace models

#endif // OPENAI_GPT_4_HPP
