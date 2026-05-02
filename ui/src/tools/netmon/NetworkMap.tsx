/**
 * Force-directed network topology visualisation built on D3.
 *
 * Renders agents and remote hosts as nodes and observed traffic as directed
 * edges. Edges between the same pair are merged so the line width can encode
 * total flow count, and the SVG supports pan / zoom plus a hover tooltip.
 */
import { useRef, useEffect, useCallback } from 'react';
import * as d3 from 'd3';
import { GraphData, GraphNode, GraphEdge } from './api';

/** D3 simulation node — extends the platform `GraphNode` with D3 layout fields. */
interface SimNode extends d3.SimulationNodeDatum {
  id: string;
  label: string;
  type: 'agent' | 'host';
  size: number;
}

/** D3 simulation link — carries traffic metadata used for tooltips and width. */
interface SimLink extends d3.SimulationLinkDatum<SimNode> {
  port: number;
  proto: string;
  count: number;
}

interface NetworkMapProps {
  /** Nodes + edges to render. */
  data: GraphData;
  /** SVG canvas width in pixels. */
  width?: number;
  /** SVG canvas height in pixels. */
  height?: number;
}

/** Force-directed graph of agents → remote hosts. */
export default function NetworkMap({ data, width = 900, height = 500 }: NetworkMapProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const tooltipRef = useRef<HTMLDivElement>(null);

  const renderGraph = useCallback(() => {
    if (!svgRef.current) return;

    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();

    if (data.nodes.length === 0) return;

    // Merge edges by source+target for thicker lines
    const edgeMap = new Map<string, { source: string; target: string; count: number; ports: string[] }>();
    data.edges.forEach((e: GraphEdge) => {
      const key = `${e.source}→${e.target}`;
      if (edgeMap.has(key)) {
        const existing = edgeMap.get(key)!;
        existing.count += e.count;
        const portLabel = `${e.port}/${e.proto}`;
        if (!existing.ports.includes(portLabel)) existing.ports.push(portLabel);
      } else {
        edgeMap.set(key, { source: e.source, target: e.target, count: e.count, ports: [`${e.port}/${e.proto}`] });
      }
    });
    const mergedEdges = Array.from(edgeMap.values());

    // Scale node sizes
    const maxSize = Math.max(...data.nodes.map((n: GraphNode) => n.size), 1);
    const nodeRadius = (n: SimNode) => {
      const base = n.type === 'agent' ? 18 : 8;
      return base + (n.size / maxSize) * 12;
    };

    // Scale edge widths
    const maxEdge = Math.max(...mergedEdges.map(e => e.count), 1);
    const edgeWidth = (count: number) => 1 + (count / maxEdge) * 5;

    const nodes: SimNode[] = data.nodes.map((n: GraphNode) => ({ ...n }));
    const links: SimLink[] = mergedEdges.map(e => ({
      source: e.source,
      target: e.target,
      count: e.count,
      port: 0,
      proto: e.ports.join(', '),
    }));

    const g = svg.append('g');

    // Zoom
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.3, 4])
      .on('zoom', (event) => g.attr('transform', event.transform));
    svg.call(zoom);

    // Arrow marker
    svg.append('defs').append('marker')
      .attr('id', 'arrowhead')
      .attr('viewBox', '0 -5 10 10')
      .attr('refX', 20)
      .attr('refY', 0)
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', '#94a3b8');

    // Force simulation
    const simulation = d3.forceSimulation(nodes)
      .force('link', d3.forceLink<SimNode, SimLink>(links).id(d => d.id).distance(120))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius((d) => nodeRadius(d as SimNode) + 5));

    // Edges
    const link = g.append('g')
      .selectAll('line')
      .data(links)
      .join('line')
      .attr('stroke', '#cbd5e1')
      .attr('stroke-width', d => edgeWidth(d.count))
      .attr('stroke-opacity', 0.6)
      .attr('marker-end', 'url(#arrowhead)');

    // Node groups
    const node = g.append('g')
      .selectAll('g')
      .data(nodes)
      .join('g')
      .call(d3.drag<SVGGElement, SimNode>()
        .on('start', (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart();
          d.fx = d.x;
          d.fy = d.y;
        })
        .on('drag', (event, d) => {
          d.fx = event.x;
          d.fy = event.y;
        })
        .on('end', (event, d) => {
          if (!event.active) simulation.alphaTarget(0);
          d.fx = null;
          d.fy = null;
        })
      );

    // Node circles
    node.append('circle')
      .attr('r', d => nodeRadius(d))
      .attr('fill', d => d.type === 'agent' ? '#3b82f6' : '#8b5cf6')
      .attr('stroke', d => d.type === 'agent' ? '#1d4ed8' : '#6d28d9')
      .attr('stroke-width', 2)
      .attr('cursor', 'grab')
      .style('filter', d => d.type === 'agent' ? 'drop-shadow(0 2px 4px rgba(59,130,246,0.4))' : 'none');

    // Agent icon (server icon for agents)
    node.filter(d => d.type === 'agent')
      .append('text')
      .attr('text-anchor', 'middle')
      .attr('dominant-baseline', 'central')
      .attr('font-size', '14px')
      .attr('fill', 'white')
      .attr('pointer-events', 'none')
      .text('🖥');

    // Host icon
    node.filter(d => d.type === 'host')
      .append('text')
      .attr('text-anchor', 'middle')
      .attr('dominant-baseline', 'central')
      .attr('font-size', '10px')
      .attr('fill', 'white')
      .attr('pointer-events', 'none')
      .text('●');

    // Labels
    node.append('text')
      .attr('dy', d => nodeRadius(d) + 14)
      .attr('text-anchor', 'middle')
      .attr('font-size', d => d.type === 'agent' ? '12px' : '10px')
      .attr('font-weight', d => d.type === 'agent' ? 'bold' : 'normal')
      .attr('fill', '#374151')
      .attr('pointer-events', 'none')
      .text(d => {
        const label = d.label;
        return label.length > 25 ? label.substring(0, 22) + '...' : label;
      });

    // Tooltip on hover
    const tooltip = d3.select(tooltipRef.current);

    node.on('mouseover', (event, d) => {
      tooltip
        .style('display', 'block')
        .style('left', `${event.offsetX + 10}px`)
        .style('top', `${event.offsetY - 10}px`)
        .html(`
          <strong>${d.label}</strong><br/>
          Type: ${d.type === 'agent' ? '🖥 Agent' : '🌐 Host'}<br/>
          Connections: ${d.size}
        `);
    })
    .on('mouseout', () => {
      tooltip.style('display', 'none');
    });

    link.on('mouseover', (event, d) => {
      const src = typeof d.source === 'object' ? (d.source as SimNode).label : d.source;
      const tgt = typeof d.target === 'object' ? (d.target as SimNode).label : d.target;
      tooltip
        .style('display', 'block')
        .style('left', `${event.offsetX + 10}px`)
        .style('top', `${event.offsetY - 10}px`)
        .html(`
          <strong>${src} → ${tgt}</strong><br/>
          Ports: ${d.proto}<br/>
          Total: ${d.count} connections
        `);
    })
    .on('mouseout', () => {
      tooltip.style('display', 'none');
    });

    // Tick
    simulation.on('tick', () => {
      link
        .attr('x1', d => (d.source as SimNode).x!)
        .attr('y1', d => (d.source as SimNode).y!)
        .attr('x2', d => (d.target as SimNode).x!)
        .attr('y2', d => (d.target as SimNode).y!);

      node.attr('transform', d => `translate(${d.x},${d.y})`);
    });

    // Initial zoom to fit
    simulation.on('end', () => {
      const bounds = (g.node() as SVGGElement)?.getBBox();
      if (bounds) {
        const padding = 40;
        const scale = Math.min(
          width / (bounds.width + padding * 2),
          height / (bounds.height + padding * 2),
          1.5
        );
        const tx = width / 2 - (bounds.x + bounds.width / 2) * scale;
        const ty = height / 2 - (bounds.y + bounds.height / 2) * scale;
        svg.transition().duration(500).call(
          zoom.transform,
          d3.zoomIdentity.translate(tx, ty).scale(scale)
        );
      }
    });
  }, [data, width, height]);

  useEffect(() => {
    renderGraph();
  }, [renderGraph]);

  return (
    <div style={{ position: 'relative', backgroundColor: 'white', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
      <svg
        ref={svgRef}
        width={width}
        height={height}
        style={{ display: 'block', width: '100%', height: `${height}px` }}
      />
      <div
        ref={tooltipRef}
        style={{
          display: 'none',
          position: 'absolute',
          backgroundColor: 'rgba(0,0,0,0.85)',
          color: 'white',
          padding: '8px 12px',
          borderRadius: '6px',
          fontSize: '12px',
          lineHeight: '1.4',
          pointerEvents: 'none',
          zIndex: 10,
          maxWidth: '250px',
        }}
      />
      {/* Legend */}
      <div style={{
        position: 'absolute', bottom: '12px', left: '12px',
        display: 'flex', gap: '16px', fontSize: '12px', color: '#6b7280',
        backgroundColor: 'rgba(255,255,255,0.9)', padding: '6px 12px', borderRadius: '6px',
      }}>
        <span><span style={{ display: 'inline-block', width: 12, height: 12, borderRadius: '50%', backgroundColor: '#3b82f6', marginRight: 4, verticalAlign: 'middle' }}></span>Agent</span>
        <span><span style={{ display: 'inline-block', width: 12, height: 12, borderRadius: '50%', backgroundColor: '#8b5cf6', marginRight: 4, verticalAlign: 'middle' }}></span>Host</span>
        <span style={{ color: '#9ca3af' }}>Scroll to zoom · Drag nodes</span>
      </div>
    </div>
  );
}
