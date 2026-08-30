package com.example

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DisplayName
import org.junit.jupiter.api.Test

class OrderKtTest {

    @Test
    // covers: SC-01
    fun `passes with backticks and spaces`() {
        assertTrue(true)
    }

    @Test
    // covers: SC-02
    fun failsDeliberately() {
        fail<Unit>("seeded deliberate failure")
    }

    @Test
    @DisplayName("pretty display name")
    // covers: SC-03
    fun displayNamed() {
        assertTrue(true)
    }
}
