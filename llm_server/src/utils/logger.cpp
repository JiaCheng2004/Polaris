#include "utils/logger.hpp"

#include <iostream>
#include <fstream>
#include <chrono>
#include <ctime>
#include <deque>
#include <mutex>
#include <vector>

// Maximum number of logs to keep in memory
static const size_t MAX_IN_MEMORY_LOGS = 4096;

namespace {
    // In-memory log buffer
    std::deque<std::string> g_inMemoryLogs;
    std::mutex g_logMutex;

    // File stream for logs
    static std::ofstream g_logFile(SERVER_LOG_PATH, std::ios::app);

    /**
     * Build a formatted log line (e.g. "[2025-02-16 13:45:00][INFO] Hello world").
     */
    std::string buildLogLine(const std::string& level, const std::string& message) {
        auto now = std::chrono::system_clock::now();
        auto timeT = std::chrono::system_clock::to_time_t(now);
    
        // Convert to local time, or gmtime if you prefer UTC
        std::tm localTm{};
    #if defined(_WIN32)
        localtime_s(&localTm, &timeT);
    #else
        localtime_r(&timeT, &localTm);
    #endif
    
        // Format: (YYYY-MM-DD HH:MM:SS)
        char timeBuf[32];
        std::strftime(timeBuf, sizeof(timeBuf), "(%Y-%m-%d %H:%M:%S)", &localTm);
    
        // Make the level bracket a fixed width: e.g. "[INFO    ]" is 10 chars
        // The level name can be left-padded or right-padded. Here's a simple approach:
        // - If level == "INFO", we want "[INFO    ]"
        // - If level == "ERROR", we want "[ERROR   ]"
        // - etc.
    
        // Let's create a small function to pad/truncate the level:
        auto padLevel = [&](const std::string& lvl) {
            // We'll ensure total 8 characters inside the brackets
            // e.g. "INFO" -> "INFO    ", "WARN" -> "WARN    ", "ERROR" -> "ERROR   "
            static const int width = 8;
            std::string padded = lvl;
            if ((int)padded.size() < width) {
                padded.append(width - padded.size(), ' '); 
            } else {
                padded = padded.substr(0, width); // truncate if longer
            }
            return "[" + padded + "]";
        };
    
        std::string levelField = padLevel(level);
    
        // Build the final line
        // e.g. "(2025-02-17 07:52:32) [INFO    ] Some message"
        std::string line;
        line.reserve(64 + message.size());
        line.append(timeBuf)      // (2025-02-17 07:52:32)
            .append(" ")
            .append(levelField)   // [INFO    ]
            .append(" ")
            .append(message);     // Some message text
    
        return line;
    }
    

    /**
     * Append a new log line to:
     *   1) console (stdout or stderr),
     *   2) in-memory buffer (up to 4096 logs),
     *   3) log file (/var/log/llm_server/server.log)
     */
    void pushLogLine(const std::string& level, const std::string& line) {
        // 1) Print to console
        if (level == "ERROR" || level == "WARN") {
            std::cerr << line << std::endl;
        } else {
            std::cout << line << std::endl;
        }

        // 2) In-memory buffer
        {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_inMemoryLogs.push_back(line);
            if (g_inMemoryLogs.size() > MAX_IN_MEMORY_LOGS) {
                g_inMemoryLogs.pop_front(); // discard oldest
            }
        }

        // 3) Write to file
        if (g_logFile.is_open()) {
            g_logFile << line << std::endl;
            // Optionally flush
            // g_logFile.flush();
        }
    }
} // anonymous namespace

namespace utils {

void Logger::info(const std::string& message) {
    std::string line = buildLogLine("INFO", message);
    pushLogLine("INFO", line);
}

void Logger::error(const std::string& message) {
    std::string line = buildLogLine("ERROR", message);
    pushLogLine("ERROR", line);
}

void Logger::warn(const std::string& message) {
    std::string line = buildLogLine("WARN", message);
    pushLogLine("WARN", line);
}

void Logger::setLogFile(const std::string& filename) {
    // Close old file if open
    if (g_logFile.is_open()) {
        g_logFile.close();
    }
    // Open new file
    g_logFile.open(filename, std::ios::app);
}

std::vector<std::string> Logger::getRecentLogs(int maxCount) {
    std::lock_guard<std::mutex> lock(g_logMutex);
    if (maxCount <= 0 || g_inMemoryLogs.empty()) {
        return {};
    }

    // If fewer logs than maxCount, return all
    size_t have = g_inMemoryLogs.size();
    size_t start = (have > (size_t)maxCount) ? have - maxCount : 0;

    // Copy to result vector
    std::vector<std::string> result;
    result.reserve(have - start);
    for (size_t i = start; i < have; ++i) {
        result.push_back(g_inMemoryLogs[i]);
    }
    return result;
}

} // namespace utils
