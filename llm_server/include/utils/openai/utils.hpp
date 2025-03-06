#ifndef UTILS_HPP
#define UTILS_HPP

#include <string>
#include <unordered_set>
#include <nlohmann/json.hpp>

namespace openai_utils
{
    /**
     * @brief Extracts file extension from a filename. Always returns lowercase with no leading dot.
     *
     * Example:
     *  getFileExtension("example.TXT") -> "txt"
     *  getFileExtension("/tmp/my.file.pdf") -> "pdf"
     *
     * @param fileName The input file name (can contain path).
     * @return The extension (without the '.'), in lowercase. If none, returns "".
     */
    std::string getFileExtension(const std::string &fileName);

    /**
     * @brief Checks if the extension is in the allowed set. The extension should be already-lowercased, no dot.
     * @param ext The extension, e.g., "pdf".
     * @param allowed A set (or vector) of allowed exts in lowercase, no dots.
     * @return true if ext is in allowed, false if not.
     */
    bool isExtensionSupported(const std::string &ext,
                              const std::vector<std::string> &allowed);
                              
    // Overload for an std::unordered_set
    bool isExtensionSupported(const std::string &ext,
                              const std::unordered_set<std::string> &allowed);
    
    /**
     * @brief Upload a file to OpenAI and get the file_id back.
     *
     * @param localFilePath Path to the local file to upload.
     * @param openAIKey The Bearer token for authorization.
     * @return file_id on success (e.g., "file-abc123"), or empty string if error.
     */
    std::string uploadFile(const std::string &localFilePath, const std::string &openAIKey);

    /**
     * @brief Create a thread with the specified messages. Returns thread_id on success.
     *
     * @param messages The JSON array of messages (transformed).
     * @param openAIKey The Bearer token for authorization.
     * @return The thread_id (e.g. "thread_abc123") on success, empty string on error.
     */
    std::string createThread(const nlohmann::json &messages, const std::string &openAIKey);

    /**
     * @brief Create a run in a thread (i.e. start the assistant). Returns run_id on success.
     *
     * @param threadId The thread_id where the run is created.
     * @param openAIKey The Bearer token for authorization.
     * @param assistantId The assistant ID (e.g. "asst_abc123").
     * @return The run_id (e.g. "run_abc123") on success, empty string on error.
     */
    std::string createRun(const std::string &threadId,
                          const std::string &openAIKey,
                          const std::string &assistantId);

    /**
     * @brief Poll the run status until completion or failure.
     *
     * @param threadId The thread to which the run belongs.
     * @param runId The run id.
     * @param openAIKey Bearer token.
     * @param outUsage On success, filled with usage->total_tokens if the API returns usage info.
     * @param maxRetries Maximum times to poll before giving up.
     * @return true if completed successfully, false if failed or timed out.
     */
    bool waitForRunCompletion(const std::string &threadId,
                              const std::string &runId,
                              const std::string &openAIKey,
                              int &outUsage,
                              int maxRetries = 120);

    /**
     * @brief Get the last message ID in a thread.
     *
     * @param threadId The thread to check.
     * @param openAIKey Bearer token.
     * @return last_id if found, empty string if error.
     */
    std::string getLastMessageId(const std::string &threadId, const std::string &openAIKey);

    /**
     * @brief Fetch a specific message by ID from a thread.
     *
     * @param threadId The thread ID.
     * @param messageId The message ID to fetch.
     * @param openAIKey Bearer token.
     * @return The entire message JSON object. On error, may return an empty object.
     */
    nlohmann::json getMessageById(const std::string &threadId,
                                  const std::string &messageId,
                                  const std::string &openAIKey);
} // namespace openai_utils

#endif // UTILS_HPP
