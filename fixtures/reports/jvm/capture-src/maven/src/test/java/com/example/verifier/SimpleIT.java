package com.example.verifier;

import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

class SimpleIT {

    @Test
    void passingTest() {
        assertEquals(4, 2 + 2);
    }

    @Test
    void failingAssertionTest() {
        assertEquals(5, 2 + 2);
    }

    @Test
    void throwingExceptionTest() {
        throw new IllegalStateException("boom");
    }

    @Disabled("intentionally skipped for fixture capture")
    @Test
    void disabledTest() {
        assertEquals(1, 1);
    }

    @Nested
    class NestedCases {

        @Test
        void nestedPassingTest() {
            assertEquals(6, 3 + 3);
        }
    }
}
