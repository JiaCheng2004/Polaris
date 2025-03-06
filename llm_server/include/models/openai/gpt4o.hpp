#ifndef GPT4O_HPP
#define GPT4O_HPP

#pragma once

#include "utils/imodel.hpp"
#include "utils/string_utils.hpp" 
#include "utils/model_result.hpp"
#include "utils/multipart_utils.hpp"
#include <nlohmann/json.hpp>
#include <unordered_set>
#include <vector>

extern nlohmann::json g_config;

namespace models
{
/**
 * @brief Concrete implementation of the IModel interface for OpenAI GPT-4.
 */
class OpenAIGPT4o : public IModel
{
public:
    /**
     * @brief Uploads file parts and queries the GPT-4o model with the provided JSON input.
     * @param input     The JSON input containing model request details.
     * @param fileParts A list of file attachments (if any).
     * @return A ModelResult containing success/failure status, error messages, and any returned data.
     */
    ModelResult uploadAndQuery(
        const nlohmann::json &input,
        const std::vector<utils::MultipartPart> &fileParts) override;
};

struct GPT4oConfig
{
    static const std::unordered_set<std::string>& getSupportedExtensions()
    {
        static const std::unordered_set<std::string> exts = {
            "c", "cpp", "css", "csv", "doc", "docx", "gif", "go", "html",
            "java", "jpeg", "jpg", "js", "json", "md", "pdf", "php", "pkl",
            "png", "pptx", "py", "rb", "tar", "tex", "ts", "txt", "webp",
            "xlsx", "xml", "zip"
        };
        return exts;
    }
};

} // namespace models

#endif // GPT4O_HPP


