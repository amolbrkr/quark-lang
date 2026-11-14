// quark/types/resource.hpp - Opaque external resource handles
//
// Every handle has the Quark type `resource`. Its kind (`file`, or a name an
// extension registered) is runtime-only. The handle object is scanned GC
// memory: it holds an optional parent value, so a connection keeps its
// database reachable, and Boehm finalizes the connection first.
//
// The slot table is the root that keeps a payload alive when that payload is
// GC memory (a file's QFilePayload). A library object's payload is a native
// pointer owned by the library; the close function frees it. The parent lives
// on the handle, not in the slot, so the registry does not keep it alive
// after the handle itself is unreachable.
//
// A close function must not allocate GC memory and must not store the handle
// anywhere. It runs once, from an explicit close or from the finalizer.
#ifndef QUARK_TYPES_RESOURCE_HPP
#define QUARK_TYPES_RESOURCE_HPP

#include "../core/value.hpp"
#include "../core/gc.hpp"
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

enum QResourceKind : uint16_t {
    QRES_KIND_NONE = 0,
    QRES_KIND_FILE = 1,
    // Kinds 2..15 are reserved for built-ins. Extension kinds start at 16.
    QRES_KIND_USER = 16,
};

enum QResourceFlags : uint16_t {
    QRES_FLAG_NONE = 0,
    QRES_FLAG_OWNED = 1 << 0,
    QRES_FLAG_CLOSED = 1 << 1,
    QRES_FLAG_FINALIZABLE = 1 << 2,
};

// Frees a native payload. Not called for a null payload. Must not allocate
// GC memory (it may run inside a finalizer).
using QResourceCloseFn = void (*)(void* payload);

struct QResourceHandle {
    uint32_t slot;
    uint32_t generation;
    uint16_t kind;
    uint16_t flags;
    QValue parent;
};

struct QResourceSlot {
    uint32_t generation;
    uint16_t kind;
    uint16_t flags;
    void* payload;
    bool occupied;
};

// The slot vector must use the GC allocator: a slot's payload (for example a
// file's QFilePayload) is GC-allocated and is referenced only from here, so a
// buffer the collector doesn't scan would let it free a live payload.
// The free list is integers, not GC pointers, and a finalizer may push to it,
// so it stays on the ordinary allocator and never calls the collector.
struct QResourceRegistry {
    std::vector<QResourceSlot, q_allocator<QResourceSlot>> slots;
    std::vector<uint32_t> free_list;
};

struct QResourceKindInfo {
    std::string name;
    QResourceCloseFn close;
};

inline QResourceRegistry& q_resource_registry() {
    static QResourceRegistry registry;
    return registry;
}

// Built-in kinds occupy ids below QRES_KIND_USER. The table is ordinary
// memory: it is filled before the collector is used, and it holds function
// pointers and names, not Quark values.
inline std::vector<QResourceKindInfo>& q_resource_kinds() {
    static std::vector<QResourceKindInfo> kinds = [] {
        std::vector<QResourceKindInfo> k;
        k.reserve(64);
        k.resize(QRES_KIND_USER);
        k[QRES_KIND_NONE].name = "resource";
        k[QRES_KIND_FILE].name = "file";
        return k;
    }();
    return kinds;
}

inline const char* q_resource_kind_name(uint16_t kind) {
    const auto& kinds = q_resource_kinds();
    if (kind < kinds.size() && !kinds[kind].name.empty()) {
        return kinds[kind].name.c_str();
    }
    return "resource";
}

inline void q_resource_set_close(uint16_t kind, QResourceCloseFn fn) {
    auto& kinds = q_resource_kinds();
    if (kind < kinds.size()) {
        kinds[kind].close = fn;
    }
}

// Registers an extension kind and returns its id (>= QRES_KIND_USER).
// `name` is copied. Defining the same name twice is a runtime error.
inline uint16_t q_define_handle_kind(const char* name, QResourceCloseFn close_fn) {
    if (name == nullptr || name[0] == '\0') {
        q_runtime_reportf("runtime error: define_handle_kind needs a non-empty name\n");
        std::exit(1);
    }
    auto& kinds = q_resource_kinds();
    for (const auto& k : kinds) {
        if (k.name == name) {
            q_runtime_reportf("runtime error: handle kind '%s' is already defined\n", name);
            std::exit(1);
        }
    }
    if (kinds.size() >= 65535) {
        q_runtime_reportf("runtime error: too many handle kinds\n");
        std::exit(1);
    }
    kinds.push_back(QResourceKindInfo{name, close_fn});
    return static_cast<uint16_t>(kinds.size() - 1);
}

inline QValue q_resource_no_parent() {
    QValue q;
    q.type = QValue::VAL_NULL;
    q.data.resource_val = nullptr;
    return q;
}

inline void q_resource_on_unreachable(QResourceHandle* handle);

#ifdef QUARK_USE_GC
inline void q_resource_arm_finalizer(QResourceHandle* handle) {
    // A non-capturing lambda is a function pointer. The finalizer runs when
    // the handle is unreachable; the parent value inside it keeps the parent
    // handle alive until this one has been finalized.
    GC_register_finalizer(
        handle,
        [](void* obj, void* cd) {
            (void)cd;
            q_resource_on_unreachable(static_cast<QResourceHandle*>(obj));
        },
        nullptr,
        nullptr,
        nullptr);
}
#else
inline void q_resource_arm_finalizer(QResourceHandle* handle) { (void)handle; }
#endif

inline QResourceHandle* q_resource_create(uint16_t kind, void* payload, uint16_t flags, QValue parent = q_resource_no_parent()) {
    QResourceRegistry& registry = q_resource_registry();

    uint32_t slotIndex = 0;
    if (!registry.free_list.empty()) {
        slotIndex = registry.free_list.back();
        registry.free_list.pop_back();

        QResourceSlot& slot = registry.slots[slotIndex];
        slot.generation += 1;
        slot.kind = kind;
        slot.flags = static_cast<uint16_t>(flags & ~QRES_FLAG_CLOSED);
        slot.payload = payload;
        slot.occupied = true;
    } else {
        slotIndex = static_cast<uint32_t>(registry.slots.size());
        registry.slots.push_back(QResourceSlot{1, kind, static_cast<uint16_t>(flags & ~QRES_FLAG_CLOSED), payload, true});
    }

    // Scanned, not atomic: `parent` may point at another handle.
    QResourceHandle* handle = static_cast<QResourceHandle*>(q_malloc(sizeof(QResourceHandle)));
    if (!handle) {
        return nullptr;
    }

    const QResourceSlot& slot = registry.slots[slotIndex];
    handle->slot = slotIndex;
    handle->generation = slot.generation;
    handle->kind = kind;
    handle->flags = slot.flags;
    handle->parent = parent;
    if ((handle->flags & QRES_FLAG_FINALIZABLE) != 0) {
        q_resource_arm_finalizer(handle);
    }
    return handle;
}

inline bool q_resource_resolve(const QValue& value, uint16_t expectedKind, QResourceSlot*& outSlot, QResourceHandle*& outHandle, const char*& err) {
    if (value.type != QValue::VAL_RESOURCE || value.data.resource_val == nullptr) {
        err = "invalid resource handle";
        return false;
    }

    QResourceHandle* handle = value.data.resource_val;
    QResourceRegistry& registry = q_resource_registry();
    if (handle->slot >= registry.slots.size()) {
        err = "invalid resource handle";
        return false;
    }

    QResourceSlot& slot = registry.slots[handle->slot];
    if (!slot.occupied) {
        err = "resource is closed";
        return false;
    }
    if (slot.generation != handle->generation) {
        err = "stale resource handle";
        return false;
    }
    if (expectedKind != QRES_KIND_NONE) {
        if (slot.kind != expectedKind || handle->kind != expectedKind) {
            err = "resource kind mismatch";
            return false;
        }
    }

    outSlot = &slot;
    outHandle = handle;
    err = nullptr;
    return true;
}

// Drops the slot. Does not run the kind's close function: callers that own
// the payload (std/io's close, which reports a failed close) do that first.
inline void q_resource_invalidate(const QResourceHandle* handle) {
    if (!handle) {
        return;
    }

    QResourceRegistry& registry = q_resource_registry();
    if (handle->slot >= registry.slots.size()) {
        return;
    }

    QResourceSlot& slot = registry.slots[handle->slot];
    if (!slot.occupied || slot.generation != handle->generation) {
        return;
    }

    slot.occupied = false;
    slot.payload = nullptr;
    slot.kind = QRES_KIND_NONE;
    slot.flags = QRES_FLAG_CLOSED;
    registry.free_list.push_back(handle->slot);
}

// Runs the kind's close function once, then drops the slot. A second call
// for the same handle does nothing.
inline void q_resource_release(const QResourceHandle* handle) {
    if (!handle) {
        return;
    }
    QResourceRegistry& registry = q_resource_registry();
    if (handle->slot >= registry.slots.size()) {
        return;
    }
    QResourceSlot& slot = registry.slots[handle->slot];
    if (!slot.occupied || slot.generation != handle->generation) {
        return;
    }
    void* payload = slot.payload;
    uint16_t kind = slot.kind;
    slot.occupied = false;
    slot.payload = nullptr;
    slot.kind = QRES_KIND_NONE;
    slot.flags = QRES_FLAG_CLOSED;
    registry.free_list.push_back(handle->slot);

    const auto& kinds = q_resource_kinds();
    if (payload != nullptr && kind < kinds.size() && kinds[kind].close != nullptr) {
        kinds[kind].close(payload);
    }
}

inline void q_resource_on_unreachable(QResourceHandle* handle) {
    if (handle == nullptr) {
        return;
    }
    if ((handle->flags & QRES_FLAG_FINALIZABLE) == 0) {
        return;
    }
    q_resource_release(handle);
}

inline bool q_resource_is_alive(QValue value) {
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    return q_resource_resolve(value, QRES_KIND_NONE, slot, handle, err);
}

#endif // QUARK_TYPES_RESOURCE_HPP
