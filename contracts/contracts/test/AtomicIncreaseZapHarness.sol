// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "../AtomicIncreaseZap.sol";

contract AtomicIncreaseZapHarness is AtomicIncreaseZap {
    function executeSwapForTest(SwapParams calldata swap) external onlyOwner {
        _executeSwap(swap);
    }

    function validateSwapForTest(SwapParams calldata swap) external pure {
        _validateSwapParams(swap);
    }
}

