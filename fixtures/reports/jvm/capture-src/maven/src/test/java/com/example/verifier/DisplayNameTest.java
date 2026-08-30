package com.example.verifier;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

@DisplayName("Custom Class Display Name")
class DisplayNameTest {

    @DisplayName("custom method display")
    @Test
    void customDisplay() {
        assertEquals(2, 1 + 1);
    }
}
