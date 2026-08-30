package com.example.verifier

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.ValueSource

class ParamTest {

    @ParameterizedTest
    @ValueSource(ints = [1, 2, 3])
    fun allPassing(value: Int) {
        assertTrue(value > 0)
    }

    @ParameterizedTest
    @ValueSource(ints = [1, 2, 3])
    fun oneFailing(value: Int) {
        assertTrue(value != 2, "value must not be 2")
    }
}
