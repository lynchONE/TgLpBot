// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

library Actions {
    // liquidity actions
    uint8 constant INCREASE_LIQUIDITY = 0x00;
    uint8 constant DECREASE_LIQUIDITY = 0x01;
    uint8 constant MINT_POSITION = 0x02;
    uint8 constant BURN_POSITION = 0x03;
    uint8 constant INCREASE_LIQUIDITY_FROM_DELTAS = 0x04;
    uint8 constant MINT_POSITION_FROM_DELTAS = 0x05;

    // swapping
    uint8 constant SWAP_EXACT_IN_SINGLE = 0x06;
    uint8 constant SWAP_EXACT_IN = 0x07;
    uint8 constant SWAP_EXACT_OUT_SINGLE = 0x08;
    uint8 constant SWAP_EXACT_OUT = 0x09;

    // donate (not supported in the position manager or router)
    uint8 constant DONATE = 0x0a;

    // settling
    uint8 constant SETTLE = 0x0b;
    uint8 constant SETTLE_ALL = 0x0c;
    uint8 constant SETTLE_PAIR = 0x0d;

    // taking
    uint8 constant TAKE = 0x0e;
    uint8 constant TAKE_ALL = 0x0f;
    uint8 constant TAKE_PORTION = 0x10;
    uint8 constant TAKE_PAIR = 0x11;

    uint8 constant CLOSE_CURRENCY = 0x12;
    uint8 constant CLEAR_OR_TAKE = 0x13;
    uint8 constant SWEEP = 0x14;

    uint8 constant WRAP = 0x15;
    uint8 constant UNWRAP = 0x16;

    // 6909 (not supported in the position manager or router)
    uint8 constant MINT_6909 = 0x17;
    uint8 constant BURN_6909 = 0x18;
}
