#include "utils/logger.hpp"

#include <chrono>
#include <ctime>
#include <deque>
#include <fstream>
#include <iostream>
#include <mutex>
#include <string>
#include <vector>

/**
 * @brief The maximum number of log entries stored in memory.
 */
static const size_t MAX_IN_MEMORY_LOGS = 4096;

namespace
{
    /**
     * @brief A thread-safe container for recent log messages.
     */
    std::deque<std::string> g_inMemoryLogs;

    /**
     * @brief A mutex to synchronize access to the in-memory log buffer.
     */
    std::mutex g_logMutex;

    /**
     * @brief A file stream used for appending log entries to disk.
     *
     * This is initialized to the default file path defined in LOGGER_HPP.
     */
    static std::ofstream g_logFile(SERVER_LOG_PATH, std::ios::app);

    /**
     * @brief Constructs a formatted log line.
     *
     * The format includes a timestamp, log level, and the message:
     * \code
     * (YYYY-MM-DD HH:MM:SS) [LEVEL   ] message text
     * \endcode
     *
     * @param level   The severity level (e.g. "INFO", "ERROR", "WARN").
     * @param message The message text.
     * @return A fully formatted log line.
     */
    std::string buildLogLine(const std::string &level, const std::string &message)
    {
        auto now = std::chrono::system_clock::now();
        auto timeT = std::chrono::system_clock::to_time_t(now);

        std::tm localTm{};
#if defined(_WIN32)
        localtime_s(&localTm, &timeT);
#else
        localtime_r(&timeT, &localTm);
#endif

        char timeBuf[32];
        std::strftime(timeBuf, sizeof(timeBuf), "(%Y-%m-%d %H:%M:%S)", &localTm);

        // Helper to pad or truncate the log level to 8 characters inside brackets
        auto padLevel = [&](const std::string &lvl) {
            static const int width = 8;
            std::string padded = lvl;
            if ((int)padded.size() < width)
            {
                padded.append(width - padded.size(), ' ');
            }
            else
            {
                padded = padded.substr(0, width);
            }
            return "[" + padded + "]";
        };

        std::string levelField = padLevel(level);

        // Build the final line
        std::string line;
        line.reserve(64 + message.size());
        line.append(timeBuf)
            .append(" ")
            .append(levelField)
            .append(" ")
            .append(message);

        return line;
    }

    /**
     * @brief Writes the log line to multiple outputs: console, in-memory buffer, and disk file.
     *
     * @param level The log severity level (e.g., "INFO", "ERROR", "WARN").
     * @param line  The formatted log line.
     */
    void pushLogLine(const std::string &level, const std::string &line)
    {
        // 1) Print to console
        if (level == "ERROR" || level == "WARN")
        {
            std::cerr << line << std::endl;
        }
        else
        {
            std::cout << line << std::endl;
        }

        // 2) Store in the in-memory buffer
        {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_inMemoryLogs.push_back(line);
            if (g_inMemoryLogs.size() > MAX_IN_MEMORY_LOGS)
            {
                g_inMemoryLogs.pop_front();
            }
        }

        // 3) Write to the log file
        if (g_logFile.is_open())
        {
            g_logFile << line << std::endl;
            // Optional: g_logFile.flush();
        }
    }

} // end anonymous namespace

namespace utils
{

void Logger::info(const std::string &message)
{
    std::string line = buildLogLine("INFO", message);
    pushLogLine("INFO", line);
}

void Logger::error(const std::string &message)
{
    std::string line = buildLogLine("ERROR", message);
    pushLogLine("ERROR", line);
}

void Logger::warn(const std::string &message)
{
    std::string line = buildLogLine("WARN", message);
    pushLogLine("WARN", line);
}

void Logger::setLogFile(const std::string &filename)
{
    if (g_logFile.is_open())
    {
        g_logFile.close();
    }
    g_logFile.open(filename, std::ios::app);
}

std::vector<std::string> Logger::getRecentLogs(int maxCount)
{
    std::lock_guard<std::mutex> lock(g_logMutex);
    if (maxCount <= 0 || g_inMemoryLogs.empty())
    {
        return {};
    }

    size_t have = g_inMemoryLogs.size();
    size_t start = (have > static_cast<size_t>(maxCount)) ? have - maxCount : 0;

    std::vector<std::string> result;
    result.reserve(have - start);
    for (size_t i = start; i < have; ++i)
    {
        result.push_back(g_inMemoryLogs[i]);
    }
    return result;
}

} // namespace utils
