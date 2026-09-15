package com.admin.common.utils;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class FlowBillingUtilTests {
    @Test
    void calculatesEveryBillingModeWithoutChangingDirections() {
        assertEquals(80L, FlowBillingUtil.calculate(120L, 80L, 1));
        assertEquals(200L, FlowBillingUtil.calculate(120L, 80L, 2));
        assertEquals(120L, FlowBillingUtil.calculate(120L, 80L, 3));
    }

    @Test
    void rejectsUnknownModeAndClampsNegativeCounters() {
        assertEquals(10L, FlowBillingUtil.calculate(-1L, 10L, 2));
        assertThrows(IllegalArgumentException.class, () -> FlowBillingUtil.calculate(1L, 1L, 4));
    }
}
