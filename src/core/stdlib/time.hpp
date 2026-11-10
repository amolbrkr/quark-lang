// stdlib/time.hpp — native implementations for the std/time module (time.qrk).
#ifndef QUARK_STDLIB_TIME_HPP
#define QUARK_STDLIB_TIME_HPP

#include <chrono>
#include <thread>

// Seconds since the Unix epoch (wall clock), with sub-second precision.
inline double q_time_now() {
    using namespace std::chrono;
    return duration<double>(system_clock::now().time_since_epoch()).count();
}

// Seconds from an arbitrary fixed point (monotonic); use differences to time code.
inline double q_time_clock() {
    using namespace std::chrono;
    return duration<double>(steady_clock::now().time_since_epoch()).count();
}

// Pause for the given number of seconds (fractions allowed; negative is 0).
inline QValue q_time_sleep(double seconds) {
    if (seconds > 0) {
        std::this_thread::sleep_for(std::chrono::duration<double>(seconds));
    }
    return qv_null();
}

#endif // QUARK_STDLIB_TIME_HPP
