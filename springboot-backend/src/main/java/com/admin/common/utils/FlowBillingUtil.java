package com.admin.common.utils;

/** Calculates quota usage while keeping directional traffic unchanged. */
public final class FlowBillingUtil {
    private FlowBillingUtil() {}

    public static long calculate(long inbound, long outbound, int mode) {
        long safeInbound = Math.max(0L, inbound);
        long safeOutbound = Math.max(0L, outbound);
        switch (mode) {
            case 1:
                return safeOutbound;
            case 2:
                return Math.addExact(safeInbound, safeOutbound);
            case 3:
                return Math.max(safeInbound, safeOutbound);
            default:
                throw new IllegalArgumentException("flow billing mode must be 1, 2 or 3");
        }
    }
}
