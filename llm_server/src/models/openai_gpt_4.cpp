#include "openai_gpt_4.hpp"
#include <iostream>
#include <fstream>
#include <filesystem> // C++17

namespace fs = std::filesystem;
namespace models {

ModelResult OpenAIGPT4::uploadAndQuery(
    const nlohmann::json& input,
    const std::vector<crow::multipart::part>& fileParts)
{
    // Create a result object (assuming ModelResult is something you define)
    ModelResult result;

    // Define or ensure /tmp directory exists (you might want to make this configurable)
    fs::path tmpPath = "/tmp/llm_server"; 

    // You can also create a new directory each time to avoid collisions.
    // For instance, create a unique directory under /tmp:
    // fs::path uniqueDir = fs::temp_directory_path() / fs::unique_path();
    // fs::create_directory(uniqueDir);

    // Let's just use /tmp for simplicity in this example.
    if (!fs::exists(tmpPath)) {
        // Optionally create it if it doesn't exist
        fs::create_directories(tmpPath);
    }

    // Iterate over each uploaded file
    for (const auto& part : fileParts) {
        // part.filename gives you the name of the uploaded file
        // part.body is the raw file contents
        // part.headers contains the raw headers (if needed)
        // part.content_type is the MIME type of the uploaded file

        // Build the destination file path
        // (Note: It's good practice to validate or sanitize the filename)
        fs::path filePath = tmpPath / part.filename;

        // Write the file to disk
        std::ofstream ofs(filePath, std::ios::binary);
        if (!ofs) {
            std::cerr << "Could not open file for writing: " << filePath << std::endl;
            // handle error
            continue;
        }

        // Write the body data
        ofs.write(part.body.data(), static_cast<std::streamsize>(part.body.size()));
        ofs.close();

        // Now you can access file metadata
        // 1. File name
        std::string fileName = part.filename;

        // 2. Content type (MIME type)
        std::string contentType = part.content_type; 

        // 3. File size - you can get from part.body.size() or from filesystem
        auto fileSizeBytes = part.body.size(); 

        // 4. Use <filesystem> to query the file info from disk if needed
        auto fileSizeFromDisk = fs::file_size(filePath);

        // Print or store this info as needed
        std::cout << "File saved to: "      << filePath      << std::endl
                  << "Original name: "      << fileName      << std::endl
                  << "Content type: "       << contentType   << std::endl
                  << "Size from memory: "   << fileSizeBytes << " bytes" << std::endl
                  << "Size from file: "     << fileSizeFromDisk << " bytes" << std::endl;

        // You can do whatever you need with the file now or later...
        // e.g., pass `filePath` to another function that processes the file
    }

    // ... do any other processing you need with the 'input' json or the saved files ...

    // Return something meaningful for your application
    return result;
}

} // namespace models
