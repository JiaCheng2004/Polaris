#ifndef GOOGLE_GEMINI2_PRO_HPP
#define GOOGLE_GEMINI2_PRO_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include <vector>

namespace models {

class GoogleGemini2Pro : public IModel {
public:
    ModelResult uploadAndQuery(const nlohmann::json& input,
                               const std::vector<crow::multipart::part>& fileParts) override;
};

} // namespace models

#endif // GOOGLE_GEMINI2_PRO_HPP
