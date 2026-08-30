package com.example.verifier;

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static org.junit.jupiter.api.Assertions.assertTrue;

class ParamTest {

    @ParameterizedTest
    @ValueSource(ints = {1, 2, 3})
    void allPassing(int value) {
        assertTrue(value > 0);
    }

    @ParameterizedTest
    @ValueSource(ints = {1, 2, 3})
    void oneFailing(int value) {
        assertTrue(value != 2, "value must not be 2");
    }
}
