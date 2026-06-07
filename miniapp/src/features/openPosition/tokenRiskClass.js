export function tokenRiskPanelClass(tone) {
    switch (tone) {
        case 'high':
        case 'critical':
            return 'border-red-500/35 bg-red-500/10 text-red-800 dark:text-red-100';
        case 'medium':
        case 'unknown':
            return 'border-amber-500/35 bg-amber-500/10 text-amber-800 dark:text-amber-100';
        default:
            return 'border-emerald-500/25 bg-emerald-500/10 text-emerald-800 dark:text-emerald-100';
    }
}
