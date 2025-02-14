#ifndef TOKEN_TRACKER_HPP
#define TOKEN_TRACKER_HPP

namespace utils {

/**
 * A minimal utility to track token usage. 
 * For demonstration, it just accumulates an integer.
 */
class TokenTracker {
public:
    static void addUsage(int count);
    static int getTotalUsage();

private:
    TokenTracker() = default;
};

} // namespace utils

#endif // TOKEN_TRACKER_HPP
