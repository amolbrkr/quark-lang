// quark/core/threads.hpp - the program's thread count (specs/tables_v0_3_spec.md
// §8.2). The thread pool (core/par.hpp) uses this many threads, the calling
// thread included; the garbage collector marks with as many.
//
//   QUARK_THREADS   set by `quark run --threads N`, read at startup; a value
//                   that isn't a whole number from 1 to q_par_max_threads is
//                   an error, not ignored
//   default         the logical CPUs the process may use: its CPU affinity,
//                   capped by its container's CPU quota (cgroup v2 cpu.max,
//                   or v1 cpu.cfs_quota_us / cpu.cfs_period_us) rounded up
//
// The REPL always uses one thread (D52).
#ifndef QUARK_CORE_THREADS_HPP
#define QUARK_CORE_THREADS_HPP

#include "diagnostics.hpp"
#include <cerrno>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <thread>

#if defined(__linux__)
#include <sched.h>
#include <sys/stat.h>
#include <unistd.h>
#elif defined(__APPLE__)
#include <unistd.h>
#elif defined(_WIN32)
extern "C" __declspec(dllimport) void* __stdcall GetCurrentProcess(void);
extern "C" __declspec(dllimport) int __stdcall GetProcessAffinityMask(void*, unsigned long long*, unsigned long long*);
#endif

inline constexpr long long q_par_max_threads = 4096;

namespace quark {
namespace par {

#if defined(__linux__)

// cgroup_ratio reads a CPU quota as quota / period from one cgroup
// directory: 0 when it sets none, -1 when the files aren't there.
inline double cgroup_v2_ratio(const std::string& dir) {
    FILE* f = std::fopen((dir + "/cpu.max").c_str(), "r");
    if (f == nullptr) return -1;
    char quota[64] = {0}, period[64] = {0};
    const int n = std::fscanf(f, "%63s %63s", quota, period);
    std::fclose(f);
    if (n < 1 || std::strcmp(quota, "max") == 0) return 0;
    const double q = std::atof(quota);
    const double p = n == 2 ? std::atof(period) : 100000.0;
    return q > 0 && p > 0 ? q / p : 0;
}

inline double cgroup_v1_ratio(const std::string& dir) {
    FILE* fq = std::fopen((dir + "/cpu.cfs_quota_us").c_str(), "r");
    if (fq == nullptr) return -1;
    long long q = -1, p = 0;
    if (std::fscanf(fq, "%lld", &q) != 1) q = -1;
    std::fclose(fq);
    FILE* fp = std::fopen((dir + "/cpu.cfs_period_us").c_str(), "r");
    if (fp != nullptr) {
        if (std::fscanf(fp, "%lld", &p) != 1) p = 0;
        std::fclose(fp);
    }
    return q > 0 && p > 0 ? static_cast<double>(q) / static_cast<double>(p) : 0;
}

inline bool is_dir(const std::string& path) {
    struct stat st;
    return ::stat(path.c_str(), &st) == 0 && S_ISDIR(st.st_mode);
}

// cgroup_walk is the smallest quota from the process's cgroup up to the
// hierarchy's root (a parent's quota caps its children). A cgroup path that
// isn't under root (a container without a cgroup namespace sees its host
// path) is skipped up to the part that is.
template <class Ratio>
inline double cgroup_walk(const std::string& root, std::string rel, Ratio ratio) {
    double best = 0;
    while (true) {
        const std::string dir = root + rel;
        if (is_dir(dir)) {
            const double r = ratio(dir);
            if (r > 0 && (best == 0 || r < best)) best = r;
        }
        if (rel.empty() || rel == "/") break;
        const size_t slash = rel.find_last_of('/');
        rel = slash == std::string::npos || slash == 0 ? std::string() : rel.substr(0, slash);
    }
    return best;
}

// cgroup_cpu_limit is the process's CPU quota in CPUs (0: none).
inline double cgroup_cpu_limit() {
    FILE* f = std::fopen("/proc/self/cgroup", "r");
    if (f == nullptr) return 0;
    char line[4096];
    std::string v2_path;
    std::string v1_path;
    bool have_v2 = false, have_v1 = false;
    while (std::fgets(line, sizeof line, f) != nullptr) {
        std::string s(line);
        while (!s.empty() && (s.back() == '\n' || s.back() == '\r')) s.pop_back();
        const size_t a = s.find(':');
        const size_t b = a == std::string::npos ? std::string::npos : s.find(':', a + 1);
        if (b == std::string::npos) continue;
        const std::string controllers = s.substr(a + 1, b - a - 1);
        const std::string path = s.substr(b + 1);
        if (s.compare(0, a, "0") == 0 && controllers.empty()) {
            v2_path = path;
            have_v2 = true;
            continue;
        }
        // v1: the line whose controller list has "cpu".
        size_t start = 0;
        while (start <= controllers.size()) {
            size_t end = controllers.find(',', start);
            if (end == std::string::npos) end = controllers.size();
            if (controllers.compare(start, end - start, "cpu") == 0) {
                v1_path = path;
                have_v1 = true;
            }
            start = end + 1;
        }
    }
    std::fclose(f);
    double limit = 0;
    if (have_v2) limit = cgroup_walk("/sys/fs/cgroup", v2_path, cgroup_v2_ratio);
    if (limit == 0 && have_v1) {
        for (const char* root : {"/sys/fs/cgroup/cpu,cpuacct", "/sys/fs/cgroup/cpu", "/sys/fs/cgroup/cpuacct,cpu"}) {
            if (!is_dir(root)) continue;
            limit = cgroup_walk(root, v1_path, cgroup_v1_ratio);
            if (limit > 0) break;
        }
    }
    return limit;
}

inline int default_threads() {
    int n = 0;
    cpu_set_t set;
    CPU_ZERO(&set);
    if (sched_getaffinity(0, sizeof set, &set) == 0) n = CPU_COUNT(&set);
    if (n <= 0) n = static_cast<int>(sysconf(_SC_NPROCESSORS_ONLN));
    const double quota = cgroup_cpu_limit();
    if (quota > 0) {
        const int q = static_cast<int>(std::ceil(quota - 1e-9));
        if (q < n) n = q;
    }
    return n < 1 ? 1 : n;
}

#elif defined(__APPLE__)

inline int default_threads() {
    const long n = sysconf(_SC_NPROCESSORS_ONLN);
    return n < 1 ? 1 : static_cast<int>(n);
}

#elif defined(_WIN32)

inline int default_threads() {
    unsigned long long proc = 0, sys = 0;
    int n = 0;
    if (GetProcessAffinityMask(GetCurrentProcess(), &proc, &sys) != 0) n = __builtin_popcountll(proc);
    if (n <= 0) n = static_cast<int>(std::thread::hardware_concurrency());
    return n < 1 ? 1 : n;
}

#else

inline int default_threads() {
    const unsigned n = std::thread::hardware_concurrency();
    return n < 1 ? 1 : static_cast<int>(n);
}

#endif

// parse_threads reads a QUARK_THREADS value: digits only, 1 to the maximum.
inline int parse_threads(const char* s) {
    if (s == nullptr || *s == '\0') return 0;
    long long v = 0;
    for (const char* p = s; *p; p++) {
        if (*p < '0' || *p > '9') return 0;
        v = v * 10 + (*p - '0');
        if (v > q_par_max_threads) return 0;
    }
    return v >= 1 ? static_cast<int>(v) : 0;
}

inline int& configured_threads() {
    static int n = 0;
    return n;
}

} // namespace par
} // namespace quark

// q_par_threads is the program's thread count: QUARK_THREADS when set, else
// the default. Read once, the first time (generated main calls q_par_init
// first, so a bad value fails before the program runs).
inline int q_par_threads() {
#if defined(QUARK_REPL)
    return 1;
#else
    int& n = quark::par::configured_threads();
    if (n > 0) return n;
    const char* env = std::getenv("QUARK_THREADS");
    if (env != nullptr) {
        const int v = quark::par::parse_threads(env);
        if (v == 0) {
            q_runtime_reportf("runtime error: QUARK_THREADS must be a whole number of threads from 1 to %lld, got '%s'\n",
                              q_par_max_threads, env);
            q_exit(1);
        }
        n = v;
        return n;
    }
    n = quark::par::default_threads();
    return n;
#endif
}

inline void q_par_init() { (void)q_par_threads(); }

// q_par_one_thread fixes the count at 1, before anything reads it: the REPL
// host, whose inputs all run on one thread.
inline void q_par_one_thread() { quark::par::configured_threads() = 1; }

#endif // QUARK_CORE_THREADS_HPP
