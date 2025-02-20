#include "server/http_server.hpp"
#include "utils/logger.hpp"

#include <fstream>
#include <iostream>
#include <nlohmann/json.hpp>
#include <stdexcept>
#include <string>

/**
 * @brief A global configuration object loaded at startup from config/config.json.
 */
nlohmann::json g_config;

/**
 * @brief The main entry point for the LLM Server application.
 *
 * This function:
 * - Loads a JSON configuration file
 * - Logs a startup message
 * - Starts the HTTP server
 * - Catches and displays any fatal errors
 *
 * @param argc The command-line argument count.
 * @param argv The command-line arguments.
 * @return An integer exit code (0 for normal termination, 1 for errors).
 */
int main(int argc, char *argv[])
{
    try
    {
        // 1) Load configuration from a JSON file
        std::ifstream ifs("config/config.json");
        if (!ifs.is_open())
        {
            throw std::runtime_error("Cannot open config/config.json");
        }
        ifs >> g_config;
        ifs.close();

        // 2) Log that we are starting
        utils::Logger::info("Starting LLM Server...");

        // 3) Start the HTTP server (this call typically blocks)
        server::startServer();
    }
    catch (const std::exception &e)
    {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}
