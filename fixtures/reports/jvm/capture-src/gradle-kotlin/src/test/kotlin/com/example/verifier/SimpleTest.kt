package com.example.verifier

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Disabled
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test

class SimpleTest {

    @Test
    fun passingTest() {
        assertEquals(4, 2 + 2)
    }

    @Test
    fun failingAssertionTest() {
        assertEquals(5, 2 + 2)
    }

    @Test
    fun throwingExceptionTest() {
        throw IllegalStateException("boom")
    }

    @Disabled("intentionally skipped for fixture capture")
    @Test
    fun disabledTest() {
        assertEquals(1, 1)
    }

    @Nested
    inner class NestedCases {

        @Test
        fun nestedPassingTest() {
            assertEquals(6, 3 + 3)
        }
    }
}
