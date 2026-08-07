import { describe, expect, it } from 'vitest'
import { reorderTabIds } from './tabOrder'

describe('reorderTabIds', () => {
  it('moves a tab after a later tab', () => {
    expect(reorderTabIds(['a', 'b', 'c', 'd'], 'b', 'c', 'after')).toEqual(['a', 'c', 'b', 'd'])
  })

  it('moves a tab before an earlier tab', () => {
    expect(reorderTabIds(['a', 'b', 'c', 'd'], 'c', 'a', 'before')).toEqual(['c', 'a', 'b', 'd'])
  })

  it('does not mutate the input or duplicate tabs', () => {
    const original = ['a', 'b', 'c']
    expect(reorderTabIds(original, 'b', 'b', 'after')).toEqual(original)
    expect(original).toEqual(['a', 'b', 'c'])
  })
})
