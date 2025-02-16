#include <crow/multipart.h>

namespace utils
{
    std::string getPartName(const crow::multipart::part& part)
    {
        // Find "Content-Disposition" in part.headers
        auto it = part.headers.find("Content-Disposition");
        if (it == part.headers.end())
            return {};

        // Instead of parsing `it->second.value`, look in `it->second.params`
        // for the key "name".
        auto nameIt = it->second.params.find("name");
        if (nameIt == it->second.params.end())
            return {};

        // That’s the field name (e.g., "json" or "files")
        return nameIt->second;
    }
} // namespace utils
