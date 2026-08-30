package com.example;

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static org.junit.jupiter.api.Assertions.assertTrue;

class ParamTest {

    @ParameterizedTest
    @ValueSource(ints = {1, 2, 3})
    // covers: SC-05
    void allPositive(int value) {
        assertTrue(value > 0);
    }
}
