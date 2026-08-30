package com.example;

import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

class OuterTest {

    @Nested
    class InnerTest {

        @Test
        // covers: SC-04
        void nestedPasses() {
            assertTrue(true);
        }
    }
}
