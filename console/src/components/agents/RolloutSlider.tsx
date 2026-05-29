'use client';

import React from 'react';

export interface RolloutSliderProps {
    /** Current canary weight percentage (0-100) */
    value: number;
    /** Callback when slider value changes */
    onChange: (value: number) => void;
    /** Predefined step buttons (e.g. [10, 30, 100]) */
    steps?: number[];
    /** Whether the slider is disabled */
    disabled?: boolean;
}

export function RolloutSlider({ value, onChange, steps = [10, 30, 100], disabled = false }: RolloutSliderProps) {
    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between">
                <label className="text-sm font-semibold text-gray-700">
                    Canary Weight
                </label>
                <span className="text-sm font-medium text-blue-600">{value}%</span>
            </div>

            {/* Slider */}
            <input
                type="range"
                min={0}
                max={100}
                step={1}
                value={value}
                onChange={(e) => onChange(Number(e.target.value))}
                disabled={disabled}
                className="h-2 w-full cursor-pointer appearance-none rounded-lg bg-gray-200 accent-blue-600 disabled:cursor-not-allowed disabled:opacity-50"
                aria-label="Canary traffic weight"
            />

            {/* Step Buttons */}
            <div className="flex gap-2">
                {steps.map((step) => (
                    <button
                        key={step}
                        type="button"
                        onClick={() => onChange(step)}
                        disabled={disabled}
                        className={`rounded border px-3 py-1 text-xs font-medium transition-colors ${value === step
                                ? 'border-blue-600 bg-blue-50 text-blue-700'
                                : 'border-gray-300 bg-white text-gray-600 hover:bg-gray-50'
                            } disabled:cursor-not-allowed disabled:opacity-50`}
                    >
                        {step}%
                    </button>
                ))}
            </div>
        </div>
    );
}
