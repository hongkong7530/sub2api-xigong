export interface MonitorRatePoint {
  rate_multiplier: number
  effective_rate_multiplier?: number
  recorded_at: string
  is_initial?: boolean
}

/** Configuration history is a step function, not interpolated prices between edits. */
export function buildMonitorRateHistory(history: MonitorRatePoint[], currentRate: number, observedAt = Date.now()) {
  const points = history.filter(p => Number.isFinite(p.rate_multiplier) && Number.isFinite(Date.parse(p.recorded_at)))
    .map((p, index) => ({ ...p, index, at: Date.parse(p.recorded_at) }))
    .sort((a, b) => a.at - b.at || b.index - a.index)
  const values = points.length ? points.map(p => p.rate_multiplier) : [currentRate]
  const finite = values.filter(Number.isFinite)
  const min = finite.length ? Math.min(...finite) : 0
  const max = finite.length ? Math.max(...finite) : 0
  const span = max - min
  const y = (value: number) => span === 0 ? 48 : 80 - (value - min) / span * 64
  const start = points[0]?.at ?? observedAt
  const end = Math.max(observedAt, points[points.length - 1]?.at ?? start, start + 1)
  const x = (at: number) => 10 + Math.max(0, Math.min(1, (at - start) / (end - start))) * 620
  let path = ''
  if (points.length) {
    path = `M10 ${y(points[0]!.rate_multiplier)}`
    for (const point of points.slice(1)) path += ` H${x(point.at)} V${y(point.rate_multiplier)}`
    path += ' H630'
  } else if (Number.isFinite(currentRate)) path = 'M10 48 H630'
  const entries = points.map((point, index) => {
    const previous = points[index - 1]?.rate_multiplier
    const delta = previous == null ? 0 : point.rate_multiplier - previous
    return { ...point, previous, delta, direction: previous == null ? (point.is_initial ? 'initial' : 'baseline') : delta < 0 ? 'decrease' : delta > 0 ? 'increase' : 'unchanged', x: x(point.at), y: y(point.rate_multiplier) }
  })
  return { path, entries, changes: entries.filter(p => p.delta !== 0).length, min, max }
}
