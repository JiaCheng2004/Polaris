#include "utils/multipart_utils.hpp"
#include <string>

namespace utils
{
    std::string getPartName(const crow::multipart::part& part)
    {
        // Look for "Content-Disposition" in part.headers (a map<string, header>)
        auto it = part.headers.find("Content-Disposition");
        if (it == part.headers.end())
        {
            // No such header
            return {};
        }

        // The actual header text is in `it->second.value`
        // e.g.  "form-data; name=\"files\"; filename=\"my.jpg\""
        const std::string& cd = it->second.value;

        // Look for name="..."
        const std::string needle = "name=\"";
        auto startPos = cd.find(needle);
        if (startPos == std::string::npos) {
            return {};
        }

        startPos += needle.size(); // move past name="

        auto endPos = cd.find('"', startPos);
        if (endPos == std::string::npos) {
            return {};
        }

        return cd.substr(startPos, endPos - startPos);
    }

} // namespace utils
