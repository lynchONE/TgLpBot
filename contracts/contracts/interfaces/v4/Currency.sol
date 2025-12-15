// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

type Currency is address;

library CurrencyLibrary {
    Currency public constant ADDRESS_ZERO = Currency.wrap(address(0));

    function unwrap(Currency currency) internal pure returns (address) {
        return Currency.unwrap(currency);
    }
}

// Helper function to unwrap Currency
function Currency_unwrap(Currency currency) pure returns (address) {
    return Currency.unwrap(currency);
}
