// quark/core/process.hpp - process-level state set by the generated main()
#ifndef QUARK_CORE_PROCESS_HPP
#define QUARK_CORE_PROCESS_HPP

// Program arguments as passed to main(). argv[0] is the executable; std/sys
// exposes the rest as sys.args().
inline int q_process_argc = 0;
inline char** q_process_argv = nullptr;

inline void q_set_args(int argc, char** argv) {
    q_process_argc = argc;
    q_process_argv = argv;
}

#endif // QUARK_CORE_PROCESS_HPP
