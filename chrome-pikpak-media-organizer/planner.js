(function initPikPakOrganizerPlanner(global) {
  const Shared = global.PikPakOrganizerShared;

  function normalizeDriveItem(raw, settings) {
    const itemId = String(raw.id || raw.file_id || raw.fileId || "");
    const name = String(raw.name || raw.file_name || raw.fileName || "");
    const parentId = String(raw.parent_id || raw.parentId || raw.scan_parent_id || "");
    const pathNames = Array.isArray(raw.scan_parent_path_names) ? raw.scan_parent_path_names.slice() : [];
    const pathIds = Array.isArray(raw.scan_parent_path_ids) ? raw.scan_parent_path_ids.slice() : [];
    const sizeValue = Number(raw.size || raw.file_size || raw.fileSize || 0);
    return {
      itemId,
      kind: String(raw.kind || ""),
      parentId,
      name,
      size: Number.isFinite(sizeValue) ? sizeValue : 0,
      mimeType: String(raw.mime_type || raw.mimeType || ""),
      createdAt: raw.created_time || raw.createdAt || raw.createdTime || "",
      addedAt:
        raw.user_modified_time ||
        raw.created_time ||
        raw.addedAt ||
        raw.added_time ||
        raw.createdAt ||
        "",
      hash: String(raw.hash || raw.file_hash || raw.sha1 || raw.md5 || ""),
      pathNames,
      pathIds,
      relativeFolderPath: Shared.buildPathKey(pathNames),
      relativeFilePath: Shared.buildPathKey(pathNames.concat(name)),
      isFolder: String(raw.kind || "") === "drive#folder",
      isVideo: Shared.isVideoItem({
        kind: raw.kind,
        mimeType: raw.mime_type || raw.mimeType,
        name
      }),
      code: Shared.extractCode(name),
      hasSubtitle: Shared.detectSubtitle(name, settings.subtitleKeywords),
      raw
    };
  }

  function normalizeScanResult(scanResult, settings) {
    const rawItems = Array.isArray(scanResult && scanResult.items) ? scanResult.items : [];
    const items = rawItems.map((item) => normalizeDriveItem(item, settings)).filter((item) => item.itemId && item.name);
    return {
      rootFolderId: String(scanResult && scanResult.rootFolderId ? scanResult.rootFolderId : "root"),
      rootParentMode: String(scanResult && scanResult.rootParentMode ? scanResult.rootParentMode : "param"),
      observedAt: String(scanResult && scanResult.observedAt ? scanResult.observedAt : ""),
      driveApiBase: String(scanResult && scanResult.driveApiBase ? scanResult.driveApiBase : ""),
      items
    };
  }

  function buildLocalPlan(normalizedScan, settings) {
    const minFileBytes = Shared.toBytesFromMegabytes(settings.minVideoSizeMb);
    const decisions = new Map();
    const duplicateGroups = [];
    const duplicateKeepIds = new Set();
    const files = normalizedScan.items.filter((item) => !item.isFolder);
    const videos = normalizedScan.items.filter((item) => item.isVideo);
    const groupsByCode = new Map();

    for (const item of videos) {
      if (!item.code) {
        continue;
      }
      const group = groupsByCode.get(item.code) || [];
      group.push(item);
      groupsByCode.set(item.code, group);
    }

    for (const [code, group] of groupsByCode.entries()) {
      if (group.length < 2) {
        continue;
      }
      const sorted = group.slice().sort(Shared.compareKeepCandidates);
      const keep = sorted[0];
      duplicateKeepIds.add(keep.itemId);
      duplicateGroups.push({
        code,
        keepItemId: keep.itemId,
        itemIds: sorted.map((item) => item.itemId)
      });
      for (let index = 1; index < sorted.length; index += 1) {
        const item = sorted[index];
        decisions.set(item.itemId, {
          itemId: item.itemId,
          action: "delete",
          reasons: [
            {
              id: "duplicate_code",
              detail: code
            }
          ]
        });
      }
    }

    for (const item of files) {
      const existing = decisions.get(item.itemId);
      if (existing && existing.action === "delete") {
        continue;
      }
      if (item.size > 0 && item.size < minFileBytes && !duplicateKeepIds.has(item.itemId)) {
        decisions.set(item.itemId, {
          itemId: item.itemId,
          action: "delete",
          reasons: [
            {
              id: "smaller_file",
              detail: Shared.formatBytes(item.size)
            }
          ]
        });
      }
    }

    const metadataCodes = Array.from(
      new Set(
        videos
          .filter((item) => {
            const decision = decisions.get(item.itemId);
            return !decision || decision.action !== "delete";
          })
          .map((item) => item.code)
          .filter(Boolean)
      )
    );

    return {
      minFileBytes,
      videos,
      decisions,
      duplicateGroups,
      duplicateKeepIds,
      metadataCodes
    };
  }

  function buildFinalPlan(normalizedScan, localPlan, metadataMap, settings) {
    const actions = [];
    const fileDeleteActions = [];
    const folderDeleteActions = [];
    const moveActions = [];
    const skipActions = [];
    const keepActions = [];
    const actionByItemId = new Map();
    const planMetadata = {
      providerHits: {},
      unresolvedCodes: []
    };

    for (const item of normalizedScan.items) {
      if (item.isFolder) {
        continue;
      }

      const localDecision = localPlan.decisions.get(item.itemId);
      if (localDecision && localDecision.action === "delete") {
        actionByItemId.set(item.itemId, buildDeleteAction(item, localDecision.reasons));
        continue;
      }

      if (!item.isVideo) {
        continue;
      }

      actionByItemId.set(item.itemId, buildVideoActionDecision(item, metadataMap, settings, planMetadata));
    }

    const sourceCleanupRoots = collapseSourceCleanupRoots(
      Array.from(actionByItemId.values())
        .filter((action) => action && action.type === "move" && action.sourceCleanupPath)
        .map((action) => action.sourceCleanupPath)
    );

    for (const item of normalizedScan.items) {
      if (item.isFolder) {
        continue;
      }

      let action = actionByItemId.get(item.itemId) || null;
      const cleanupRoot = findSourceCleanupRoot(item.relativeFolderPath, sourceCleanupRoots);
      if (cleanupRoot && (!action || (action.type !== "move" && action.type !== "keep"))) {
        action = buildDeleteAction(item, [
          {
            id: "source_folder_cleanup",
            detail: cleanupRoot
          }
        ]);
      } else if (!action) {
        action = buildSkipAction(item, "non_video");
      }

      actions.push(action);
      if (action.type === "delete") {
        fileDeleteActions.push(action);
      } else if (action.type === "move") {
        moveActions.push(action);
      } else if (action.type === "keep") {
        keepActions.push(action);
      } else if (action.type === "skip") {
        skipActions.push(action);
      }
    }

    for (const action of buildFolderCleanupActions(normalizedScan, actions)) {
      actions.push(action);
      folderDeleteActions.push(action);
    }

    const targetFolders = Array.from(
      new Set(
        moveActions
          .map((action) => action.targetPathKey)
          .filter(Boolean)
      )
    ).sort((left, right) => left.localeCompare(right));
    const folderDeletePaths = folderDeleteActions
      .map((action) => action.currentPath)
      .filter(Boolean)
      .sort((left, right) => left.localeCompare(right));
    const sourceCleanupRootList = sourceCleanupRoots
      .slice()
      .sort((left, right) => left.localeCompare(right));
    const videoDeleteCount = fileDeleteActions.filter((action) => action.isVideo).length;
    const otherFileDeleteCount = Math.max(0, fileDeleteActions.length - videoDeleteCount);

    return {
      createdAt: new Date().toISOString(),
      rootFolderId: normalizedScan.rootFolderId,
      rootParentMode: normalizedScan.rootParentMode,
      driveApiBase: normalizedScan.driveApiBase,
      observedAt: normalizedScan.observedAt,
      summary: {
        totalActions: actions.length,
        deleteCount: fileDeleteActions.length + folderDeleteActions.length,
        fileDeleteCount: fileDeleteActions.length,
        videoDeleteCount,
        otherFileDeleteCount,
        folderDeleteCount: folderDeleteActions.length,
        moveCount: moveActions.length,
        skipCount: skipActions.length,
        keepCount: keepActions.length,
        duplicateGroupCount: localPlan.duplicateGroups.length,
        targetFolderCount: targetFolders.length,
        sourceFolderCount: sourceCleanupRootList.length
      },
      metadata: planMetadata,
      targetFolders,
      sourceCleanupRoots: sourceCleanupRootList,
      folderDeletePaths,
      actions,
      deleteActions: fileDeleteActions,
      folderDeleteActions,
      moveActions,
      skipActions,
      keepActions
    };
  }

  function buildVideoActionDecision(item, metadataMap, settings, planMetadata) {
    const year = Shared.getYearFromTimestamp(item.addedAt || item.createdAt);
    if (!year) {
      return buildSkipAction(item, "no_year");
    }

    const overrideName = item.code ? settings.codeActressOverrides[item.code] || "" : "";
    const metadata = item.code ? metadataMap[item.code] || null : null;
    const actressNameZh = Shared.sanitizeFolderName(overrideName || (metadata && metadata.actressNameZh) || "");
    const metadataSource = overrideName ? "manual_override" : metadata && metadata.source ? metadata.source : "";
    if (metadataSource) {
      planMetadata.providerHits[metadataSource] = (planMetadata.providerHits[metadataSource] || 0) + 1;
    } else if (item.code && !planMetadata.unresolvedCodes.includes(item.code)) {
      planMetadata.unresolvedCodes.push(item.code);
    }

    const targetSegments = actressNameZh ? [year, actressNameZh] : [year];
    const targetPathKey = Shared.buildPathKey(targetSegments);
    const currentPathKey = item.relativeFolderPath;
    const reasons = [];
    if (!item.code) {
      reasons.push({
        id: "no_code",
        label: Shared.reasonLabel("no_code"),
        detail: ""
      });
    }
    if (!actressNameZh) {
      reasons.push({
        id: "no_actress_zh",
        label: Shared.reasonLabel("no_actress_zh"),
        detail: "宸叉寜骞翠唤鍏滃簳"
      });
    }

    if (targetPathKey === currentPathKey) {
      return {
        type: "keep",
        itemId: item.itemId,
        code: item.code,
        name: item.name,
        size: item.size,
        currentPath: item.relativeFilePath,
        targetPath: Shared.buildDisplayPath(targetSegments.concat(item.name)),
        reasons: [
          {
            id: "already_sorted",
            label: Shared.reasonLabel("already_sorted"),
            detail: ""
          },
          ...reasons
        ],
        actressNameZh,
        metadataSource
      };
    }

    return {
      type: "move",
      itemId: item.itemId,
      code: item.code,
      name: item.name,
      size: item.size,
      currentPath: item.relativeFilePath,
      targetSegments,
      targetPathKey,
      targetPath: Shared.buildDisplayPath(targetSegments.concat(item.name)),
      actressNameZh,
      metadataSource,
      sourceCleanupPath: currentPathKey || "",
      reasons
    };
  }

  function buildDeleteAction(item, reasons) {
    return {
      type: "delete",
      itemId: item.itemId,
      code: item.code,
      name: item.name,
      size: item.size,
      isVideo: item.isVideo,
      currentPath: item.relativeFilePath,
      reasons: (reasons || []).map((reason) => ({
        id: reason.id,
        label: Shared.reasonLabel(reason.id),
        detail: reason.detail || ""
      }))
    };
  }

  function collapseSourceCleanupRoots(paths) {
    const sortedPaths = Array.from(new Set((paths || []).filter(Boolean))).sort((left, right) => {
      const leftDepth = left.split("/").length;
      const rightDepth = right.split("/").length;
      return leftDepth - rightDepth || left.localeCompare(right);
    });
    const result = [];
    for (const path of sortedPaths) {
      if (result.some((existing) => path === existing || path.startsWith(existing + "/"))) {
        continue;
      }
      result.push(path);
    }
    return result;
  }

  function findSourceCleanupRoot(folderPath, cleanupRoots) {
    const currentPath = String(folderPath || "");
    if (!currentPath) {
      return "";
    }

    let matchedRoot = "";
    for (const root of cleanupRoots || []) {
      if (currentPath === root || currentPath.startsWith(root + "/")) {
        if (root.length > matchedRoot.length) {
          matchedRoot = root;
        }
      }
    }
    return matchedRoot;
  }

  function buildFolderCleanupActions(normalizedScan, itemActions) {
    const folders = normalizedScan.items.filter((item) => item.isFolder);
    if (!folders.length) {
      return [];
    }

    const actionByItemId = new Map();
    for (const action of itemActions) {
      actionByItemId.set(action.itemId, action);
    }

    const foldersById = new Map();
    const childFolderIdsByParent = new Map();
    for (const folder of folders) {
      foldersById.set(folder.itemId, folder);
      const childFolderIds = childFolderIdsByParent.get(folder.parentId) || [];
      childFolderIds.push(folder.itemId);
      childFolderIdsByParent.set(folder.parentId, childFolderIds);
    }

    const occupiedFolderPaths = new Set();
    for (const item of normalizedScan.items) {
      if (item.isFolder) {
        continue;
      }
      const action = actionByItemId.get(item.itemId);
      if (!action || (action.type !== "move" && action.type !== "delete")) {
        if (item.relativeFolderPath) {
          occupiedFolderPaths.add(item.relativeFolderPath);
        }
      }
    }

    for (const action of itemActions) {
      if (action.type !== "move" || !Array.isArray(action.targetSegments)) {
        continue;
      }
      const segments = [];
      for (const segment of action.targetSegments) {
        segments.push(segment);
        const pathKey = Shared.buildPathKey(segments);
        if (pathKey) {
          occupiedFolderPaths.add(pathKey);
        }
      }
    }

    const folderStateCache = new Map();
    function folderWillRemain(folderId) {
      if (folderStateCache.has(folderId)) {
        return folderStateCache.get(folderId);
      }

      const folder = foldersById.get(folderId);
      if (!folder) {
        folderStateCache.set(folderId, false);
        return false;
      }

      let result = occupiedFolderPaths.has(folder.relativeFilePath);
      if (!result) {
        const childFolderIds = childFolderIdsByParent.get(folderId) || [];
        result = childFolderIds.some((childId) => folderWillRemain(childId));
      }
      folderStateCache.set(folderId, result);
      return result;
    }

    return folders
      .filter((folder) => !folderWillRemain(folder.itemId))
      .sort((left, right) => {
        const depthDiff = right.pathNames.length - left.pathNames.length;
        return depthDiff || right.relativeFilePath.localeCompare(left.relativeFilePath);
      })
      .map((folder) => ({
        type: "delete",
        itemId: folder.itemId,
        code: "",
        name: folder.name,
        size: 0,
        isFolder: true,
        depth: folder.pathNames.length + 1,
        currentPath: folder.relativeFilePath,
        reasons: [
          {
            id: "empty_folder",
            label: Shared.reasonLabel("empty_folder"),
            detail: ""
          }
        ]
      }));
  }

  function buildSkipAction(item, reasonId, extra) {
    return {
      type: "skip",
      itemId: item.itemId,
      code: item.code,
      name: item.name,
      size: item.size,
      currentPath: item.relativeFilePath,
      reasons: [
        {
          id: reasonId,
          label: Shared.reasonLabel(reasonId),
          detail: ""
        }
      ],
      ...(extra || {})
    };
  }

  global.PikPakOrganizerPlanner = Object.freeze({
    buildFinalPlan,
    buildLocalPlan,
    normalizeScanResult
  });
})(globalThis);
