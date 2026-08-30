package com.example.verifier

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BacktickTest {

    @Test
    fun `passes with backticks and spaces`() {
        assertTrue(true)
    }

    @Test
    fun `alsoBackticked`() {
        assertTrue(true)
    }
}
