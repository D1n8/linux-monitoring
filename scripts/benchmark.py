#!/usr/bin/env python3
"""Measure real container counters; never infer CPU from one docker stats sample.

Uses an existing running Compose deployment. Measures stopped collection, then one
and five simulated dashboard clients. Restores the previous collection state.
It does not restart containers, erase data, or generate artificial host load.
"""
import argparse
import contextlib
import datetime
import http.client
import json
import math
import pathlib
import platform
import socket
import statistics
import subprocess
import threading
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
BASE = 'http://127.0.0.1:9099/api/'

class DockerConnection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect('/var/run/docker.sock')

def engine(path):
    connection = DockerConnection('localhost', timeout=8)
    try:
        connection.request('GET', path)
        response = connection.getresponse()
        if response.status != 200:
            raise RuntimeError(f'Docker returned {response.status}: {response.read(300)!r}')
        return json.load(response)
    finally:
        connection.close()

def api(path, enabled=None):
    request = urllib.request.Request(BASE + path)
    if enabled is not None:
        request = urllib.request.Request(BASE + path, data=json.dumps({'enabled': enabled}).encode(),
                                         headers={'Content-Type': 'application/json'}, method='PUT')
    with urllib.request.urlopen(request, timeout=16) as response:
        return json.load(response)

def container_stats(identifier):
    data = engine(f'/containers/{identifier}/stats?stream=false&one-shot=true')
    memory = data.get('memory_stats', {})
    inactive = memory.get('stats', {}).get('inactive_file', memory.get('stats', {}).get('total_inactive_file', 0))
    io = data.get('blkio_stats', {}).get('io_service_bytes_recursive') or []
    return {'cpu_ns': data['cpu_stats']['cpu_usage']['total_usage'],
            'working_set_bytes': max(0, memory.get('usage', 0) - inactive),
            'read_bytes': sum(x['value'] for x in io if x['op'].lower() == 'read'),
            'write_bytes': sum(x['value'] for x in io if x['op'].lower() == 'write')}

def percentile(values, quantile):
    ordered = sorted(values)
    return ordered[min(len(ordered)-1, max(0, math.ceil(quantile * len(ordered))-1))]

class Dashboard:
    def __init__(self):
        self.connection = urllib.request.urlopen(BASE + 'stream', timeout=20)
        self.stop = threading.Event()
        self.reader = threading.Thread(target=self.consume, daemon=True)
        self.poller = threading.Thread(target=self.poll, daemon=True)
        self.reader.start(); self.poller.start()
    def consume(self):
        try:
            while not self.stop.is_set():
                if not self.connection.readline():
                    break
        except (OSError, ValueError, http.client.HTTPException):
            pass
    def poll(self):
        tick = 0
        while not self.stop.is_set():
            try:
                api('host'); api('health')
                if tick % 3 == 0:
                    api('history')
            except (OSError, ValueError, http.client.HTTPException):
                pass
            tick += 1
            self.stop.wait(10)
    def close(self):
        self.stop.set()
        # Closing the socket also removes the SSE subscriber immediately.
        with contextlib.suppress(OSError, AttributeError):
            self.connection.fp.raw._sock.shutdown(socket.SHUT_RDWR)
        self.reader.join(timeout=2); self.poller.join(timeout=2)
        self.connection.close()

def measure(enabled, viewers, seconds, identifiers):
    api('service', enabled)
    dashboards = [Dashboard() for _ in range(viewers)]
    try:
        time.sleep(3)  # connection setup and the first history query are outside the window
        initial_health = api('health')
        initial = {name: container_stats(identifier) for name, identifier in identifiers.items()}
        memories = {name: [] for name in identifiers}
        viewer_counts = [initial_health['viewers']]
        collected_ms = []
        latencies = []
        begin = time.monotonic()
        deadline = begin + seconds
        final = {key: value.copy() for key, value in initial.items()}
        while True:
            for name, identifier in identifiers.items():
                final[name] = container_stats(identifier)
                memories[name].append(final[name]['working_set_bytes'])
            t = time.perf_counter()
            snapshot = api('snapshot')
            viewer_counts.append(api('health')['viewers'])
            latencies.append((time.perf_counter() - t) * 1000)
            if enabled:
                collected_ms.append(snapshot['collection_ms'])
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            time.sleep(min(5, remaining))
        elapsed = time.monotonic() - begin
        final_health = api('health')
        resources = {}
        for name in identifiers:
            resources[name] = {
                'cpu_percent_one_core': (final[name]['cpu_ns'] - initial[name]['cpu_ns']) / 1e9 / elapsed * 100,
                'working_set_mib_mean': statistics.mean(memories[name]) / 1024**2,
                'working_set_mib_peak': max(memories[name]) / 1024**2,
                'block_read_bytes_delta': final[name]['read_bytes'] - initial[name]['read_bytes'],
                'block_write_bytes_delta': final[name]['write_bytes'] - initial[name]['write_bytes']}
        result = {'scenario': 'disabled' if not enabled else f'live_{viewers}_viewers', 'monitoring_enabled': enabled, 'requested_viewers': viewers,
                  'actual_viewers': final_health['viewers'], 'viewers_min': min(viewer_counts), 'viewers_max': max(viewer_counts), 'simulated_streams_alive': all(d.reader.is_alive() for d in dashboards), 'elapsed_seconds': elapsed, 'containers': resources,
                  'collected_delta': final_health['collected'] - initial_health['collected'],
                  'dropped_delta': final_health['dropped'] - initial_health['dropped'],
                  'missed_delta': final_health['missed'] - initial_health['missed'],
                  'snapshot_ms_p50': statistics.median(latencies), 'snapshot_ms_p95': percentile(latencies, .95)}
        if collected_ms:
            result['collection_ms_p50'] = statistics.median(collected_ms)
            result['collection_ms_p95'] = percentile(collected_ms, .95)
        print(json.dumps(result, ensure_ascii=False), flush=True)
        return result
    finally:
        for dashboard in dashboards:
            dashboard.close()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--seconds', type=int, default=60, help='measurement seconds per scenario (30..3600)')
    parser.add_argument('--output', type=pathlib.Path, default=pathlib.Path('/tmp/goinmetrics-performance.json'))
    args = parser.parse_args()
    if not 30 <= args.seconds <= 3600:
        parser.error('--seconds must be between 30 and 3600')
    identifiers = {name: subprocess.check_output(['docker', 'compose', 'ps', '-q', name], cwd=ROOT, text=True).strip() for name in ['app', 'postgres']}
    if not all(identifiers.values()):
        parser.error('start the service with ./start.sh first')
    initial = api('health')['monitoring_enabled']
    report = {'time_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'kernel': platform.release(), 'architecture': platform.machine(),
              'cpu_count': __import__('os').cpu_count(),
              'mem_total_kib': next(int(line.split()[1]) for line in pathlib.Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemTotal:')),
              'method': 'Cumulative Docker CPU counters, working set = usage - inactive_file, sample every 5 s; one CPU core = 100%. SSE clients poll host/health every 10 s and history every 30 s. Snapshot probe every 5 s. No artificial workload; no claim of global minimum.',
              'scenarios': []}
    try:
        for enabled, viewers in [(False, 0), (True, 1), (True, 5)]:
            report['scenarios'].append(measure(enabled, viewers, args.seconds, identifiers))
    finally:
        api('service', initial)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(f'Report saved: {args.output}', flush=True)

if __name__ == '__main__':
    main()
