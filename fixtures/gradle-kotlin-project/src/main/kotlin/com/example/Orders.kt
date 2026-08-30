package com.example

object Orders {
    fun isValidQuantity(qty: Int): Boolean {
        if (qty <= 0) {
            return false
        }
        return true
    }
}
