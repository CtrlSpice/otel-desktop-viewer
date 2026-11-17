// LayerChart 2.5.2 exports this utility but omits its declaration from the package.
// Its options and result are the same contract exposed by the Dagre component.
declare module 'layerchart/utils/graph/dagre' {
  import type { DagreGraphData, DagreProps } from 'layerchart/graph'

  export function dagreGraph(
    data: DagreGraphData,
    options?: Omit<DagreProps, 'data' | 'children' | 'graph'>
  ): NonNullable<DagreProps['graph']>
}
