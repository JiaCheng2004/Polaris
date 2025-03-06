#ifndef STRING_UTILS_HPP
#define STRING_UTILS_HPP

#include <string>
#include <algorithm>

namespace utils
{
    /**
     * @brief Check if 'str' ends with 'suffix'.
     */
    inline bool endsWith(const std::string &str, const std::string &suffix)
    {
        if (suffix.size() > str.size()) return false;
        return std::equal(suffix.rbegin(), suffix.rend(), str.rbegin());
    }
}

#endif // STRING_UTILS_HPP
