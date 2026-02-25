function normalize(value) {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

function toScreamingSnake(input) {
  return String(input || "")
    .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
    .replace(/[^A-Za-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .toUpperCase();
}

function getNetworkPrefixes(networkName) {
  switch (networkName) {
    case "base":
      return ["BASE"];
    case "baseSepolia":
      return ["BASE_SEPOLIA", "BASE"];
    case "bsc":
      return ["BSC"];
    case "bscTestnet":
      return ["BSC_TESTNET", "BSC"];
    default:
      return [toScreamingSnake(networkName)];
  }
}

function usesGlobalFallback(networkName) {
  switch (networkName) {
    case "base":
    case "baseSepolia":
      return false;
    default:
      return true;
  }
}

function readEnvForNetwork(networkName, key, options = {}) {
  const allowGlobalFallback =
    typeof options.allowGlobalFallback === "boolean"
      ? options.allowGlobalFallback
      : usesGlobalFallback(networkName);
  const prefixes = getNetworkPrefixes(networkName);
  for (const prefix of prefixes) {
    const value = normalize(process.env[`${prefix}_${key}`]);
    if (value) {
      return value;
    }
  }
  if (allowGlobalFallback) {
    return normalize(process.env[key]);
  }
  return undefined;
}

function readFirstEnvForNetwork(networkName, keys, options = {}) {
  for (const key of keys) {
    const value = readEnvForNetwork(networkName, key, options);
    if (value) {
      return value;
    }
  }
  return undefined;
}

function readZapAddressForNetwork(networkName) {
  return readFirstEnvForNetwork(networkName, ["ZAP_V3_ADDRESS", "ZAP_V4_ADDRESS"]);
}

function uniqueAddressList(addresses) {
  const map = new Map();
  for (const address of addresses) {
    const value = normalize(address);
    if (!value) {
      continue;
    }
    map.set(value.toLowerCase(), value);
  }
  return [...map.values()];
}

function resolveV3ManagersForNetwork(networkName) {
  const primaryPrefix = getNetworkPrefixes(networkName)[0];

  if (networkName === "base" || networkName === "baseSepolia") {
    const explicit = readEnvForNetwork(networkName, "V3_POSITION_MANAGER_ADDRESS");
    const uniswap = readEnvForNetwork(networkName, "UNISWAP_V3_NPM_ADDRESS");
    const aerodrome = readEnvForNetwork(networkName, "AERODROME_V3_NPM_ADDRESS");
    const primary = explicit || uniswap || aerodrome;
    const extras = uniqueAddressList([uniswap, aerodrome]).filter(
      (item) => !primary || item.toLowerCase() !== primary.toLowerCase()
    );

    return {
      family: "base",
      primary,
      extras,
      details: {
        uniswap,
        aerodrome,
      },
      requiredHints: [
        `${primaryPrefix}_V3_POSITION_MANAGER_ADDRESS`,
        `${primaryPrefix}_UNISWAP_V3_NPM_ADDRESS`,
        `${primaryPrefix}_AERODROME_V3_NPM_ADDRESS`,
      ],
    };
  }

  const explicit = readEnvForNetwork(networkName, "V3_POSITION_MANAGER_ADDRESS");
  const pancake = readEnvForNetwork(networkName, "PANCAKE_V3_NPM_ADDRESS");
  const uniswap = readEnvForNetwork(networkName, "UNISWAP_V3_NPM_ADDRESS");
  const primary = explicit || pancake || uniswap;
  const extras = uniqueAddressList([pancake, uniswap]).filter(
    (item) => !primary || item.toLowerCase() !== primary.toLowerCase()
  );

  return {
    family: "bsc_like",
    primary,
    extras,
    details: {
      pancake,
      uniswap,
    },
    requiredHints: [
      `${primaryPrefix}_V3_POSITION_MANAGER_ADDRESS`,
      `${primaryPrefix}_PANCAKE_V3_NPM_ADDRESS`,
      `${primaryPrefix}_UNISWAP_V3_NPM_ADDRESS`,
      "V3_POSITION_MANAGER_ADDRESS",
      "PANCAKE_V3_NPM_ADDRESS",
      "UNISWAP_V3_NPM_ADDRESS",
    ],
  };
}

function resolveTrustedConfigForNetwork(networkName) {
  const primaryPrefix = getNetworkPrefixes(networkName)[0];
  const okxRouter = readEnvForNetwork(networkName, "OKX_SWAP_ROUTER");
  const okxApprove = readEnvForNetwork(networkName, "OKX_TOKEN_APPROVE_ADDRESS");
  const v3 = resolveV3ManagersForNetwork(networkName);
  const v4pm = readFirstEnvForNetwork(networkName, [
    "UNISWAP_V4_POSITION_MANAGER_ADDRESS",
    "V4_POSITION_MANAGER_ADDRESS",
  ]);

  const missingHints = [];
  if (!okxRouter) {
    missingHints.push(`${primaryPrefix}_OKX_SWAP_ROUTER`);
    if (usesGlobalFallback(networkName)) {
      missingHints.push("OKX_SWAP_ROUTER");
    }
  }
  if (!okxApprove) {
    missingHints.push(`${primaryPrefix}_OKX_TOKEN_APPROVE_ADDRESS`);
    if (usesGlobalFallback(networkName)) {
      missingHints.push("OKX_TOKEN_APPROVE_ADDRESS");
    }
  }
  if (!v3.primary) {
    missingHints.push(...v3.requiredHints);
  }

  return {
    okxRouter,
    okxApprove,
    v3Primary: v3.primary,
    v3Extras: v3.extras,
    v4pm,
    family: v3.family,
    missingHints: [...new Set(missingHints)],
  };
}

function getExplorerApiKeyForNetwork(networkName) {
  switch (networkName) {
    case "base":
    case "baseSepolia":
      return normalize(process.env.BASESCAN_API_KEY) || normalize(process.env.ETHERSCAN_API_KEY);
    case "bsc":
    case "bscTestnet":
      return normalize(process.env.BSCSCAN_API_KEY);
    default:
      return (
        normalize(process.env.ETHERSCAN_API_KEY) ||
        normalize(process.env.BASESCAN_API_KEY) ||
        normalize(process.env.BSCSCAN_API_KEY)
      );
  }
}

function isTruthyEnv(name) {
  const value = normalize(process.env[name]);
  if (!value) {
    return false;
  }
  const normalized = value.toLowerCase();
  return normalized === "1" || normalized === "true" || normalized === "yes" || normalized === "on";
}

function getNativeSymbolForNetwork(networkName) {
  switch (networkName) {
    case "bsc":
    case "bscTestnet":
      return "BNB";
    default:
      return "ETH";
  }
}

module.exports = {
  getNetworkPrefixes,
  usesGlobalFallback,
  readEnvForNetwork,
  readFirstEnvForNetwork,
  readZapAddressForNetwork,
  resolveV3ManagersForNetwork,
  resolveTrustedConfigForNetwork,
  getExplorerApiKeyForNetwork,
  isTruthyEnv,
  getNativeSymbolForNetwork,
};
