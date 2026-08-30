package com.example;

public final class Orders {

    private Orders() {}

    public static boolean isValidQuantity(int qty) {
        if (qty <= 0) {
            return false;
        }
        return true;
    }
}
