/**
 * Module page for the URL Monitor tool.
 *
 * Lets the operator add URLs, trigger on-demand HTTP checks, view the latest
 * status code per URL, and inspect a per-URL history log when one is selected.
 */
import { listURLs, addURL, checkURL, getHistory, deleteURL } from './api';
import { useState, useEffect } from 'react';

/** Local mirror of the API `URL` type (kept independent from the global URL ctor). */
interface URL {
  id: number;
  url: string;
  status: number | null;
  last_check: number | null;
  created_at: number;
  updated_at: number;
}

/** Local mirror of the API `HistoryEntry` type. */
interface HistoryEntry {
  id: number;
  status: number;
  response_time: number;
  error?: string;
  checked_at: number;
}

/** Page component for the URL Monitor module. */
export default function URLCheckPage() {
  const [urls, setUrls] = useState<URL[]>([]);
  const [newURL, setNewURL] = useState('');
  const [loading, setLoading] = useState(false);
  const [selectedURL, setSelectedURL] = useState<number | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>([]);

  useEffect(() => {
    loadURLs();
  }, []);

  const loadURLs = async () => {
    setLoading(true);
    try {
      const data = await listURLs();
      setUrls(data || []);
    } catch (err) {
      console.error('Failed to load URLs:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleAddURL = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newURL.trim()) return;

    try {
      await addURL(newURL);
      setNewURL('');
      loadURLs();
    } catch (err) {
      console.error('Failed to add URL:', err);
    }
  };

  const handleCheckURL = async (id: number) => {
    try {
      await checkURL(id);
      loadURLs();
      await loadHistory(id);
    } catch (err) {
      console.error('Failed to check URL:', err);
    }
  };

  const handleDeleteURL = async (id: number) => {
    if (!confirm('Delete this URL?')) return;
    try {
      await deleteURL(id);
      loadURLs();
      if (selectedURL === id) setSelectedURL(null);
    } catch (err) {
      console.error('Failed to delete URL:', err);
    }
  };

  const loadHistory = async (id: number) => {
    try {
      const data = await getHistory(id);
      setHistory(data || []);
    } catch (err) {
      console.error('Failed to load history:', err);
    }
  };

  const getStatusColor = (status: number | null) => {
    if (status === null) return 'text-gray-500';
    if (status >= 200 && status < 300) return 'text-green-600';
    if (status >= 300 && status < 400) return 'text-blue-600';
    if (status >= 400 && status < 500) return 'text-yellow-600';
    return 'text-red-600';
  };

  const getStatusLabel = (status: number | null) => {
    if (status === null) return 'Not checked';
    if (status >= 200 && status < 300) return 'OK';
    if (status >= 300 && status < 400) return 'Redirect';
    if (status >= 400 && status < 500) return 'Client Error';
    return 'Server Error';
  };

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-lg shadow p-6">
        <h1 className="text-2xl font-bold mb-4">URL Monitor</h1>

        <form onSubmit={handleAddURL} className="mb-6">
          <div className="flex gap-2">
            <input
              type="url"
              placeholder="https://example.com"
              value={newURL}
              onChange={(e) => setNewURL(e.target.value)}
              className="flex-1 px-3 py-2 border border-gray-300 rounded"
              required
            />
            <button
              type="submit"
              disabled={loading}
              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              Add URL
            </button>
          </div>
        </form>

        {loading && <p className="text-gray-500">Loading...</p>}

        {urls.length === 0 ? (
          <p className="text-gray-500">No URLs yet. Add one to get started.</p>
        ) : (
          <div className="space-y-2">
            {urls.map((url) => (
              <div
                key={url.id}
                className="flex items-center justify-between p-3 border border-gray-200 rounded hover:bg-gray-50 cursor-pointer"
                onClick={() => {
                  setSelectedURL(url.id);
                  loadHistory(url.id);
                }}
              >
                <div className="flex-1">
                  <div className="font-mono text-sm break-all">{url.url}</div>
                  <div className="text-xs text-gray-500 mt-1">
                    Last checked: {url.last_check ? new Date(url.last_check * 1000).toLocaleString() : 'Never'}
                  </div>
                </div>
                <div className={`text-lg font-bold ${getStatusColor(url.status)}`}>
                  {url.status !== null ? url.status : '-'}
                </div>
                <div className="text-sm text-gray-600 ml-4 min-w-20">{getStatusLabel(url.status)}</div>
                <div className="flex gap-2 ml-4">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleCheckURL(url.id);
                    }}
                    className="px-2 py-1 text-sm bg-green-600 text-white rounded hover:bg-green-700"
                  >
                    Check
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDeleteURL(url.id);
                    }}
                    className="px-2 py-1 text-sm bg-red-600 text-white rounded hover:bg-red-700"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {selectedURL && history.length > 0 && (
        <div className="bg-white rounded-lg shadow p-6">
          <h2 className="text-xl font-bold mb-4">Check History</h2>
          <div className="space-y-2">
            {history.map((entry) => (
              <div key={entry.id} className="flex justify-between items-center p-2 border border-gray-200 rounded text-sm">
                <div>
                  <span className={`font-bold ${getStatusColor(entry.status)}`}>{entry.status}</span>
                  {entry.error && <span className="text-red-600 ml-2">{entry.error}</span>}
                </div>
                <div className="text-gray-600">
                  {entry.response_time}ms • {new Date(entry.checked_at * 1000).toLocaleString()}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
