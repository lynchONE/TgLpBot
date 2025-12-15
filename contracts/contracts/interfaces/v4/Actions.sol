// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

library Actions {
    // Core actions
    uint8 constant SWAP_EXACT_IN_SINGLE = 0x00;
    uint8 constant SWAP_EXACT_IN = 0x01;
    uint8 constant SWAP_EXACT_OUT_SINGLE = 0x02;
    uint8 constant SWAP_EXACT_OUT = 0x03;
    
    // Liquidity actions
    uint8 constant MINT_POSITION = 0x02;  // Note: This might overlap with SWAP_EXACT_OUT_SINGLE
    uint8 constant INCREASE_LIQUIDITY = 0x03;
    uint8 constant DECREASE_LIQUIDITY = 0x04;
    uint8 constant BURN_POSITION = 0x05;
    
    // Token actions
    uint8 constant TAKE_PAIR = 0x06;
    uint8 constant SETTLE_PAIR = 0x0d;  // 13 in decimal
    uint8 constant SETTLE = 0x0b;       // 11 in decimal
    uint8 constant TAKE = 0x0e;         // 14 in decimal
    uint8 constant CLOSE_CURRENCY = 0x09;
    uint8 constant SWEEP = 0x0f;
}
