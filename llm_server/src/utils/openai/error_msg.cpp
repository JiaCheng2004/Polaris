#include "utils/openai/error_msg.hpp"

namespace openai_error
{
    std::string formatNotAllowedError(const std::string &badExt,
                                      const std::unordered_set<std::string> &allowed)
    {
        std::string msg = "Upload failed because the file format \"." + badExt
            + "\" is not allowed. Please use one of the supported file extensions: ";

        bool first = true;
        for (auto &a : allowed)
        {
            if (!first) msg += ", ";
            msg += "\"." + a + "\"";
            first = false;
        }
        return msg;
    }
}
