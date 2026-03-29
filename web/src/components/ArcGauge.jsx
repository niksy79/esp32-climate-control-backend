// 0° = top (12 o'clock), clockwise
function polarToCartesian(cx, cy, r, deg) {
  const rad = ((deg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) }
}

// Draw a clockwise arc from startAngle spanning sweepDeg degrees
function describeArc(cx, cy, r, startAngle, sweepDeg) {
  const p1 = polarToCartesian(cx, cy, r, startAngle)
  const p2 = polarToCartesian(cx, cy, r, startAngle + sweepDeg)
  const largeArc = sweepDeg > 180 ? 1 : 0
  return `M ${p1.x} ${p1.y} A ${r} ${r} 0 ${largeArc} 1 ${p2.x} ${p2.y}`
}

// Gap at bottom (6 o'clock = 180°), 110° wide for button space
const GAP_DEG = 110
const ARC_SWEEP = 360 - GAP_DEG   // 250
const ARC_START = 180 + GAP_DEG / 2 // 235 (bottom-left)
const CX = 100
const CY = 96
const R = 86

const trackPath = describeArc(CX, CY, R, ARC_START, ARC_SWEEP)

export default function ArcGauge({ value, min, max, step, unit, color, label, onIncrement, onDecrement }) {
  const pct = Math.max(0, Math.min(1, (value - min) / (max - min)))
  const valueSweep = ARC_SWEEP * pct
  const valuePath = valueSweep > 0.5 ? describeArc(CX, CY, R, ARC_START, valueSweep) : null

  // Knob position
  const knobAngle = ARC_START + valueSweep
  const knob = polarToCartesian(CX, CY, R, knobAngle)

  const formatted = typeof value === 'number'
    ? (Number.isInteger(step) && step >= 1 ? value.toFixed(0) : value.toFixed(1))
    : value

  return (
    <div className="arc-gauge">
      <svg viewBox="0 0 200 220" className="arc-gauge-svg">
        {/* Background track */}
        <path
          d={trackPath}
          fill="none"
          className="arc-gauge-track"
          strokeWidth="8"
          strokeLinecap="round"
        />
        {/* Value arc */}
        {valuePath && (
          <path
            d={valuePath}
            fill="none"
            style={{ stroke: color }}
            strokeWidth="8"
            strokeLinecap="round"
          />
        )}
        {/* Knob */}
        <circle
          cx={knob.x}
          cy={knob.y}
          r="11"
          style={{ fill: color }}
          className="arc-gauge-knob"
          strokeWidth="3"
        />
        {/* Center text */}
        <text x={CX} y={CY - 20} textAnchor="middle" className="arc-gauge-label">
          {label}
        </text>
        <text x={CX} y={CY + 14} textAnchor="middle" dominantBaseline="central" className="arc-gauge-number" style={{ fill: color }}>
          {formatted}<tspan dx="3" dy="-14" className="arc-gauge-unit">{unit}</tspan>
        </text>
      </svg>
      {/* +/- buttons in the gap */}
      <button
        type="button"
        className="arc-gauge-btn arc-gauge-btn-minus"
        onClick={onDecrement}
        aria-label="Намали"
      >
        −
      </button>
      <button
        type="button"
        className="arc-gauge-btn arc-gauge-btn-plus"
        onClick={onIncrement}
        aria-label="Увеличи"
      >
        +
      </button>
    </div>
  )
}
