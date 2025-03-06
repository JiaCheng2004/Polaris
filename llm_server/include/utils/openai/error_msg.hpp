#ifndef ERROR_MSG_HPP
#define ERROR_MSG_HPP

#pragma once

#include <string>
#include <unordered_set>

namespace openai_error
{
    /**
     * @brief Returns an error string for an unsupported extension
     * 
     * Example:
     *   Upload failed because the file format ".mov" is not allowed. 
     *   Please use one of the supported file extensions: "c", "cpp", ...
     *
     * @param ext         The extension that is not allowed
     * @param allAllowed  The list of all allowed exts in quotes.
     * @return A full descriptive error message.
     */
    std::string formatNotAllowedError(const std::string &badExt,
                                      const std::unordered_set<std::string> &allowed);
}

#endif