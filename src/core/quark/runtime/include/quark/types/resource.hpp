// quark/types/resource.hpp - Opaque external resource handles
#ifndef QUARK_TYPES_RESOURCE_HPP
#define QUARK_TYPES_RESOURCE_HPP

#include "../core/value.hpp"
#include "../core/gc.hpp"
#include <cstdint>
#include <vector>

enum QResourceKind : uint16_t {
    QRES_KIND_NONE = 0,
    QRES_KIND_FILE = 1,
};

enum QResourceFlags : uint16_t {
    QRES_FLAG_NONE = 0,
    QRES_FLAG_OWNED = 1 << 0,
    QRES_FLAG_CLOSED = 1 << 1,
    QRES_FLAG_FINALIZABLE = 1 << 2,
};

struct QResourceHandle {
    uint32_t slot;
    uint32_t generation;
    uint16_t kind;
    uint16_t reserved;
};

struct QResourceSlot {
    uint32_t generation;
    uint16_t kind;
    uint16_t flags;
    void* payload;
    bool occupied;
};

struct QResourceRegistry {
    std::vector<QResourceSlot> slots;
    std::vector<uint32_t> free_list;
};

inline QResourceRegistry& q_resource_registry() {
    static QResourceRegistry registry;
    return registry;
}

inline const char* q_resource_kind_name(uint16_t kind) {
    switch (kind) {
        case QRES_KIND_FILE:
            return "file_handle";
        default:
            return "resource";
    }
}

inline QResourceHandle* q_resource_create(uint16_t kind, void* payload, uint16_t flags) {
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

    QResourceHandle* handle = static_cast<QResourceHandle*>(q_malloc_atomic(sizeof(QResourceHandle)));
    if (!handle) {
        return nullptr;
    }

    const QResourceSlot& slot = registry.slots[slotIndex];
    handle->slot = slotIndex;
    handle->generation = slot.generation;
    handle->kind = kind;
    handle->reserved = 0;
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

inline bool q_resource_is_alive(QValue value) {
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    return q_resource_resolve(value, QRES_KIND_NONE, slot, handle, err);
}

#endif // QUARK_TYPES_RESOURCE_HPP
