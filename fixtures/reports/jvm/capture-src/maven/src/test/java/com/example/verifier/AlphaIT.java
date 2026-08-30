package com.example.verifier;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

class AlphaIT {

    @Test
    void sharedMethod() {
        assertEquals("alpha", "alpha");
    }
}
