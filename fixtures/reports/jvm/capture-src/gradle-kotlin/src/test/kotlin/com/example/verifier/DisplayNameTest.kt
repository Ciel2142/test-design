package com.example.verifier

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DisplayName
import org.junit.jupiter.api.Test

@DisplayName("Custom Class Display Name")
class DisplayNameTest {

    @DisplayName("custom method display")
    @Test
    fun customDisplay() {
        assertEquals(2, 1 + 1)
    }
}
