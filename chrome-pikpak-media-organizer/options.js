const Shared = globalThis.PikPakOrganizerShared;

const fields = {
  recursiveScan: document.getElementById("recursiveScan"),
  yearOnlyFallbackEnabled: document.getElementById("yearOnlyFallbackEnabled"),
  requireChineseActressName: document.getElementById("requireChineseActressName"),
  minVideoSizeMb: document.getElementById("minVideoSizeMb"),
  providerTimeoutMs: document.getElementById("providerTimeoutMs"),
  maxMetadataConcurrency: document.getElementById("maxMetadataConcurrency"),
  metadataCacheTtlHours: document.getElementById("metadataCacheTtlHours"),
  metadataProviders: document.getElementById("metadataProviders"),
  subtitleKeywords: document.getElementById("subtitleKeywords"),
  codeActressOverrides: document.getElementById("codeActressOverrides"),
  statusLabel: document.getElementById("statusLabel"),
  saveBtn: document.getElementById("saveBtn"),
  resetBtn: document.getElementById("resetBtn"),
  exportBtn: document.getElementById("exportBtn"),
  importInput: document.getElementById("importInput")
};

let autoSaveTimer = null;

document.addEventListener("DOMContentLoaded", async () => {
  const settings = await getSettings();
  renderSettings(settings);
  bindAutoSave();
});

fields.saveBtn.addEventListener("click", async () => {
  const settings = collectSettingsFromForm();
  await saveSettings(settings);
  fields.statusLabel.textContent = "设置已保存。";
});

fields.resetBtn.addEventListener("click", async () => {
  const response = await chrome.runtime.sendMessage({
    type: "reset-settings"
  });
  if (!response || !response.ok) {
    throw new Error(response && response.error ? response.error : "重置失败");
  }
  renderSettings(response.settings);
  fields.statusLabel.textContent = "已恢复默认设置。";
});

fields.exportBtn.addEventListener("click", async () => {
  const settings = collectSettingsFromForm();
  downloadJson("pikpak-organizer-settings.json", settings);
  fields.statusLabel.textContent = "设置已导出。";
});

fields.importInput.addEventListener("change", async (event) => {
  const file = event.target.files && event.target.files[0];
  if (!file) {
    return;
  }
  const text = await file.text();
  const parsed = JSON.parse(text);
  const settings = Shared.normalizeSettings(parsed);
  renderSettings(settings);
  await saveSettings(settings);
  fields.statusLabel.textContent = "设置已导入并保存。";
  fields.importInput.value = "";
});

async function getSettings() {
  const response = await chrome.runtime.sendMessage({
    type: "get-settings"
  });
  if (!response || !response.ok) {
    throw new Error(response && response.error ? response.error : "读取设置失败");
  }
  return response.settings;
}

async function saveSettings(settings) {
  const response = await chrome.runtime.sendMessage({
    type: "save-settings",
    settings
  });
  if (!response || !response.ok) {
    throw new Error(response && response.error ? response.error : "保存设置失败");
  }
  renderSettings(response.settings);
}

function bindAutoSave() {
  const watchedFields = [
    fields.recursiveScan,
    fields.yearOnlyFallbackEnabled,
    fields.requireChineseActressName,
    fields.minVideoSizeMb,
    fields.providerTimeoutMs,
    fields.maxMetadataConcurrency,
    fields.metadataCacheTtlHours,
    fields.metadataProviders,
    fields.subtitleKeywords,
    fields.codeActressOverrides
  ];
  for (const field of watchedFields) {
    if (!field) {
      continue;
    }
    field.addEventListener("change", queueAutoSave);
    if (field.tagName === "TEXTAREA" || field.tagName === "INPUT") {
      field.addEventListener("input", queueAutoSave);
    }
  }
}

function queueAutoSave() {
  fields.statusLabel.textContent = "设置已修改，正在自动保存...";
  clearTimeout(autoSaveTimer);
  autoSaveTimer = setTimeout(() => {
    void autoSaveForm();
  }, 300);
}

async function autoSaveForm() {
  try {
    const settings = collectSettingsFromForm();
    await saveSettings(settings);
    fields.statusLabel.textContent = "设置已自动保存。";
  } catch (error) {
    fields.statusLabel.textContent = "自动保存失败：" + (error instanceof Error ? error.message : String(error));
  }
}

function renderSettings(settings) {
  const normalized = Shared.normalizeSettings(settings);
  fields.recursiveScan.checked = normalized.recursiveScan;
  fields.yearOnlyFallbackEnabled.checked = normalized.yearOnlyFallbackEnabled;
  fields.requireChineseActressName.checked = normalized.requireChineseActressName;
  fields.minVideoSizeMb.value = String(normalized.minVideoSizeMb);
  fields.providerTimeoutMs.value = String(normalized.providerTimeoutMs);
  fields.maxMetadataConcurrency.value = String(normalized.maxMetadataConcurrency);
  fields.metadataCacheTtlHours.value = String(normalized.metadataCacheTtlHours);
  fields.metadataProviders.value = normalized.metadataProviders.join(",");
  fields.subtitleKeywords.value = normalized.subtitleKeywords.join("\n");
  fields.codeActressOverrides.value = JSON.stringify(normalized.codeActressOverrides, null, 2);
}

function collectSettingsFromForm() {
  const subtitleKeywords = fields.subtitleKeywords.value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const metadataProviders = fields.metadataProviders.value
    .split(",")
    .map((line) => line.trim())
    .filter(Boolean);

  let overrides = {};
  const overrideText = fields.codeActressOverrides.value.trim();
  if (overrideText) {
    overrides = JSON.parse(overrideText);
  }

  return Shared.normalizeSettings({
    recursiveScan: fields.recursiveScan.checked,
    yearOnlyFallbackEnabled: fields.yearOnlyFallbackEnabled.checked,
    requireChineseActressName: fields.requireChineseActressName.checked,
    minVideoSizeMb: Number(fields.minVideoSizeMb.value),
    providerTimeoutMs: Number(fields.providerTimeoutMs.value),
    maxMetadataConcurrency: Number(fields.maxMetadataConcurrency.value),
    metadataCacheTtlHours: Number(fields.metadataCacheTtlHours.value),
    metadataProviders,
    subtitleKeywords,
    codeActressOverrides: overrides
  });
}

function downloadJson(filename, data) {
  const blob = new Blob([JSON.stringify(data, null, 2)], {
    type: "application/json"
  });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
