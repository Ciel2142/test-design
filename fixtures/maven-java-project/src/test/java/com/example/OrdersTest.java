package com.example;

import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.fail;

class OrdersTest {

    @Test
    // covers: SC-01
    void passesDeliberately() {
        assertEquals(4, 2 + 2);
    }

    @Test
    // covers: SC-02
    void failsDeliberately() {
        fail("seeded deliberate failure");
    }

    @Test
    @Disabled("seeded deliberate skip")
    // covers: SC-03
    void skippedDeliberately() {
        fail("never runs");
    }

    @Test
    // covers: SC-07
    void validatesQuantityDeliberately() {
        assertTrue(Orders.isValidQuantity(3));
        assertFalse(Orders.isValidQuantity(0));
    }
}
