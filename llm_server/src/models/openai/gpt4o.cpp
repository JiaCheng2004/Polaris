#include "models/openai/gpt4o.hpp"
#include "utils/logger.hpp"
#include "utils/openai/utils.hpp"
#include "utils/openai/error_msg.hpp"
#include "utils/string_utils.hpp"
#include <unordered_map>
#include <filesystem>
#include <fstream>
#include <cstdlib>
#include <string>
#include <vector>
#include <thread>
#include <chrono>

namespace fs = std::filesystem;

namespace models
{

/**
 * @brief The main method that checks file extensions, then proceeds with your logic.
 */
ModelResult OpenAIGPT4o::uploadAndQuery(
    const nlohmann::json &input,
    const std::vector<utils::MultipartPart> &fileParts)
{
    utils::Logger::info("[OpenAIGPT4o::uploadAndQuery] Called. Number of file parts: "
                        + std::to_string(fileParts.size()));

    ModelResult result;
    result.model_used = "gpt4o";

    // Get OPENAI_API_KEY from environment
    const char* envKey = std::getenv("OPENAI_API_KEY");
    if (!envKey)
    {
        utils::Logger::error("OPENAI_API_KEY environment variable not set.");
        result.success = false;
        result.code    = 500;
        result.message = "OPENAI_API_KEY not found in environment.";
        return result;
    }
    std::string openAIKey = envKey;

    const auto &allowedExts = GPT4oConfig::getSupportedExtensions();

    fs::path tmpPath = "/tmp/llm_server";
    for (const auto &part : fileParts)
    {
        std::string ext = openai_utils::getFileExtension(part.filename);

        // Check if extension is supported
        if (!openai_utils::isExtensionSupported(ext, allowedExts))
        {
            std::string errMsg = openai_error::formatNotAllowedError(ext, allowedExts);
            utils::Logger::error("[OpenAIGPT4o::uploadAndQuery] " + errMsg);
            result.success = false;
            result.code = 400;
            result.message = errMsg;
            return result;
        }

        // If extension is good, we proceed to save the file
        if (!fs::exists(tmpPath))
        {
            fs::create_directories(tmpPath);
        }
        fs::path filePath = tmpPath / part.filename;
        std::ofstream ofs(filePath, std::ios::binary);
        if (!ofs)
        {
            utils::Logger::error("[OpenAIGPT4o::uploadAndQuery] Could not open file for writing: "
                                 + filePath.string());
            result.success = false;
            result.code = 500;
            result.message = "Could not write file: " + filePath.string();
            return result;
        }
        ofs.write(part.body.data(), static_cast<std::streamsize>(part.body.size()));
        ofs.close();

        utils::Logger::info("[OpenAIGPT4o::uploadAndQuery] Saved file: "
                            + filePath.string() + " | size: "
                            + std::to_string(part.body.size()));
    }

    std::unordered_map<std::string, std::string> uploadedFiles;

    // 4) Transform the JSON 
    nlohmann::json modified = input;
    if (!modified.contains("messages") || !modified["messages"].is_array())
    {
        result.success = false;
        result.code = 400;
        result.message = "Missing 'messages' array in input JSON.";
        return result;
    }

    // For each message: update content (image_file) and attachments
    for (auto &message : modified["messages"])
    {
        // 4a) content
        if (message.contains("content") && message["content"].is_array())
        {
            for (auto &cnt : message["content"])
            {
                if (cnt.contains("type") && cnt["type"] == "image_file")
                {
                    if (!cnt["image_file"].contains("uuid") ||
                        !cnt["image_file"].contains("original_filename")) {
                        continue;
                    }

                    std::string uuid     = cnt["image_file"]["uuid"].get<std::string>();
                    std::string origName = cnt["image_file"]["original_filename"].get<std::string>();

                    // Check if we already uploaded this 'uuid'
                    if (uploadedFiles.find(uuid) != uploadedFiles.end())
                    {
                        // Already uploaded. Reuse the file_id.
                        std::string existingFileId = uploadedFiles[uuid];
                        cnt["image_file"] = { {"file_id", existingFileId} };
                        utils::Logger::info("[uploadFile] Skipping re-upload for content image. UUID: " + uuid
                                            + " => file_id: " + existingFileId);
                        continue;
                    }

                    fs::path oldPath = tmpPath / uuid;
                    fs::path newPath = tmpPath / origName;

                    try {
                        if (fs::exists(oldPath) && oldPath != newPath)
                            fs::rename(oldPath, newPath);

                        std::string fileId = openai_utils::uploadFile(newPath.string(), openAIKey);
                        if (!fileId.empty())
                        {
                            uploadedFiles[uuid] = fileId;
                            fs::remove(newPath);
                            cnt["image_file"] = { {"file_id", fileId} };
                            result.file_ids.push_back(fileId);
                        }
                    }
                    catch (const fs::filesystem_error &ex)
                    {
                        utils::Logger::error(std::string("Rename/Remove error: ") + ex.what());
                    }
                }
            }
        }

        // 4b) attachments
        if (message.contains("attachments") && message["attachments"].is_array())
        {
            nlohmann::json newAttachments = nlohmann::json::array();
            for (auto &att : message["attachments"])
            {
                if (!att.contains("uuid") || !att.contains("original_filename")) {
                    continue;
                }
                std::string uuid     = att["uuid"].get<std::string>();
                std::string origName = att["original_filename"].get<std::string>();

                fs::path oldPath = tmpPath / uuid;
                fs::path newPath = tmpPath / origName;

                // naive check for image
                bool isImage = (utils::endsWith(origName, ".jpg") ||
                                utils::endsWith(origName, ".jpeg")||
                                utils::endsWith(origName, ".png") ||
                                utils::endsWith(origName, ".gif") ||
                                utils::endsWith(origName, ".webp"));
                                
                auto it = uploadedFiles.find(uuid);
                if (it != uploadedFiles.end())
                {
                    // Already uploaded => skip re-upload
                    std::string existingFileId = it->second;
                    if (isImage)
                    {
                        // remove from attachments
                        utils::Logger::info("Skipping re-upload for attachment image. Removing " + origName);
                        // do nothing (exclude from newAttachments)
                    }
                    else
                    {
                        // It's a non-image file, re-use the same file_id
                        nlohmann::json replaced;
                        replaced["file_id"] = existingFileId;
                        replaced["tools"]   = nlohmann::json::array();
                        replaced["tools"].push_back({{"type","file_search"}});
                        newAttachments.push_back(replaced);

                        utils::Logger::info("Skipping re-upload for attachment. Re-used file_id: " + existingFileId);
                    }
                    continue; // proceed to next attachment
                }
                
                try {
                    if (fs::exists(oldPath) && oldPath != newPath)
                        fs::rename(oldPath, newPath);

                    std::string fileId = openai_utils::uploadFile(newPath.string(), openAIKey);
                    if (!fileId.empty())
                    {
                        uploadedFiles[uuid] = fileId;
                        fs::remove(newPath);

                        if (isImage)
                        {
                            // The prompt says if it's an image typed file, remove it from attachments
                            utils::Logger::info("Removed image from attachments array: " + origName);
                        }
                        else
                        {
                            // replace with { "file_id": ..., "tools":[ {"type":"file_search"} ] }
                            nlohmann::json replaced;
                            replaced["file_id"] = fileId;
                            replaced["tools"] = nlohmann::json::array();
                            replaced["tools"].push_back({{"type","file_search"}});
                            newAttachments.push_back(replaced);
                            result.file_ids.push_back(fileId);
                        }
                    }
                }
                catch (const fs::filesystem_error &ex)
                {
                    utils::Logger::error(std::string("Rename/Remove error: ") + ex.what());
                }
            }
            message["attachments"] = newAttachments;
        }
    }

    // 5) Create thread with the newly transformed messages
    nlohmann::json msgs = modified["messages"];
    std::string threadId = openai_utils::createThread(msgs, openAIKey);
    if (threadId.empty())
    {
        result.success = false;
        result.code = 500;
        result.message = "Failed to create thread at OpenAI.";
        return result;
    }

    // 6) Create a run
    if (!g_config.contains("openai") || 
        !g_config["openai"].contains("gpt4o") ||
        !g_config["openai"]["gpt4o"].contains("assistant_id") ||
        !g_config["openai"]["gpt4o"]["assistant_id"].is_string())
    {
        throw std::runtime_error("No valid 'assistant_id' found in config!");
    }

    std::string assistantId = g_config["openai"]["gpt4o"]["assistant_id"].get<std::string>();

    utils::Logger::info("[OpenAIGPT4o::uploadAndQuery] Using assistantId: " + assistantId);

    std::string runId = openai_utils::createRun(threadId, openAIKey, assistantId);
    if (runId.empty())
    {
        result.success = false;
        result.code = 500;
        result.message = "Failed to create run at OpenAI.";
        return result;
    }

    // 7) Poll run status
    int tokenUsage = 0;
    bool done = openai_utils::waitForRunCompletion(threadId, runId, openAIKey, tokenUsage);
    if (!done)
    {
        result.success = false;
        result.code = 500;
        result.message = "Run did not complete successfully.";
        return result;
    }

    // 8) Get last message ID
    std::string lastId = openai_utils::getLastMessageId(threadId, openAIKey);
    if (lastId.empty())
    {
        result.success = false;
        result.code = 500;
        result.message = "Failed to retrieve last message ID.";
        return result;
    }

    // 9) Retrieve last message, parse the text
    auto lastMsg = openai_utils::getMessageById(threadId, lastId, openAIKey);
    if (lastMsg.empty())
    {
        result.success = false;
        result.code = 500;
        result.message = "Failed to retrieve final assistant message.";
        return result;
    }

    std::string finalText;
    if (lastMsg.contains("content") && lastMsg["content"].is_array())
    {
        for (auto &c : lastMsg["content"])
        {
            if (c.contains("type") && c["type"] == "text"
                && c.contains("text")
                && c["text"].contains("value"))
            {
                finalText += c["text"]["value"].get<std::string>() + "\n";
            }
        }
    }

    // 10) Populate the result
    result.success = true;
    result.code = 200;
    result.message = "";
    result.result = finalText;
    result.token_usage = tokenUsage;

    utils::Logger::info("[OpenAIGPT4o::uploadAndQuery] Completed processing for openai-gpt4.");
    return result;
}

} // namespace models
