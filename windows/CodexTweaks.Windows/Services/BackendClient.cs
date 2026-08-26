using System.Collections.Concurrent;
using System.Diagnostics;
using System.Reflection;
using System.Text;
using System.Text.Json;
using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using Windows.System.UserProfile;

namespace CodexTweaks.Windows.Services;

internal sealed class BackendClient : IAsyncDisposable
{
    internal const int ProtocolVersion = 9;
    private static readonly Encoding Utf8NoBom = new UTF8Encoding(encoderShouldEmitUTF8Identifier: false);

    private readonly ConcurrentDictionary<long, TaskCompletionSource<JsonElement>> _pending = new();
    private readonly SemaphoreSlim _writeLock = new(1, 1);
    private readonly JsonSerializerOptions _json = new()
    {
        PropertyNameCaseInsensitive = true,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    };
    private Process? _process;
    private long _nextId;
    private Task? _stdoutTask;
    private Task? _stderrTask;
    private bool _stopping;

    internal event Action<BackendAppSnapshot>? SnapshotChanged;
    internal event Action<string>? BackendFailed;

    internal async Task<BackendAppSnapshot> StartAsync()
    {
        if (_process is not null)
        {
            return await RequestAsync<BackendAppSnapshot>("getState", null);
        }

        var executable = Environment.GetEnvironmentVariable("CODEX_TWEAKS_BACKEND_PATH");
        if (string.IsNullOrWhiteSpace(executable))
        {
            executable = Path.Combine(AppContext.BaseDirectory, "codex-tweaks-backend.exe");
        }
        if (!File.Exists(executable))
        {
            throw new FileNotFoundException(
                PresentationFallback.Text(PresentationTextKey.AppBackendMissing),
                executable);
        }

        var start = new ProcessStartInfo(executable)
        {
            WorkingDirectory = AppContext.BaseDirectory,
            UseShellExecute = false,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
            WindowStyle = ProcessWindowStyle.Hidden,
            StandardInputEncoding = Utf8NoBom,
            StandardOutputEncoding = Utf8NoBom,
            StandardErrorEncoding = Utf8NoBom,
        };
        _process = new Process { StartInfo = start, EnableRaisingEvents = true };
        _process.Exited += (_, _) => HandleExit();
        if (!_process.Start())
        {
            _process.Dispose();
            _process = null;
            throw new InvalidOperationException(
                PresentationFallback.Text(PresentationTextKey.AppBackendNotRunning));
        }

        _stdoutTask = ReadStdoutAsync(_process);
        _stderrTask = ReadStderrAsync(_process);

        var ping = await RequestAsync<BackendPing>("ping", null);
        if (ping.ProtocolVersion != ProtocolVersion || ping.Backend != "go")
        {
            throw new InvalidOperationException(
                PresentationFallback.Text(PresentationTextKey.AppProtocolMismatch));
        }

        var local = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
        var applicationSupport = Environment.GetEnvironmentVariable("CODEX_TWEAKS_APPLICATION_SUPPORT") ?? local;
        var cache = Environment.GetEnvironmentVariable("CODEX_TWEAKS_CACHE_DIRECTORY") ?? local;
        var informational = Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion;
        var version = informational?.Split('+')[0] ?? "0.0.0-dev";
        var buildNumber = Assembly.GetExecutingAssembly()
            .GetCustomAttributes<AssemblyMetadataAttribute>()
            .FirstOrDefault(attribute => attribute.Key == "CodexTweaksBuildNumber")?.Value ?? "1";
        return await RequestAsync<BackendAppSnapshot>(
            "initialize",
            new
            {
                applicationSupportDirectory = applicationSupport,
                cacheDirectory = cache,
                bundledPackagesDirectory = Path.Combine(AppContext.BaseDirectory, "Tweaks", "packages"),
                skillPath = Path.Combine(AppContext.BaseDirectory, "Skills", "develop-codex-tweaks-package", "SKILL.md"),
                preferredLanguages = GlobalizationPreferences.Languages,
                currentVersion = version,
                buildNumber,
            });
    }

    internal async Task SendAsync(string method, object? parameters = null)
    {
        _ = await RequestAsync<JsonElement>(method, parameters);
    }

    internal async Task<BackendAppSnapshot> CheckAppUpdateAsync(bool startCheck)
    {
        if (startCheck)
        {
            await SendAsync("checkAppUpdate", new { prompt = false });
        }

        var startedAt = Stopwatch.GetTimestamp();
        while (true)
        {
            var snapshot = await RequestAsync<BackendAppSnapshot>("getState", null);
            if (!snapshot.Update.Checking)
            {
                return snapshot;
            }
            if (Stopwatch.GetElapsedTime(startedAt) >= TimeSpan.FromSeconds(45))
            {
                throw new TimeoutException(
                    PresentationFallback.Text(PresentationTextKey.AppBackendRequestFailed));
            }
            await Task.Delay(250);
        }
    }

    internal async Task<T> RequestAsync<T>(string method, object? parameters)
    {
        var process = _process ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.AppBackendNotRunning));
        var id = Interlocked.Increment(ref _nextId);
        var completion = new TaskCompletionSource<JsonElement>(TaskCreationOptions.RunContinuationsAsynchronously);
        if (!_pending.TryAdd(id, completion))
        {
            throw new InvalidOperationException(
                PresentationFallback.Text(PresentationTextKey.AppBackendRequestCreateFailed));
        }

        try
        {
            var payload = JsonSerializer.Serialize(new { id, method, @params = parameters }, _json);
            await _writeLock.WaitAsync();
            try
            {
                await process.StandardInput.WriteLineAsync(payload);
                await process.StandardInput.FlushAsync();
            }
            finally
            {
                _writeLock.Release();
            }

            var result = await completion.Task.WaitAsync(TimeSpan.FromMinutes(3));
            if (typeof(T) == typeof(JsonElement))
            {
                return (T)(object)result;
            }
            return result.Deserialize<T>(_json)
                ?? throw new InvalidOperationException(
                    PresentationFallback.Text(PresentationTextKey.AppBackendMalformed));
        }
        finally
        {
            _pending.TryRemove(id, out _);
        }
    }

    internal async Task<string> ReadAuthoringPromptAsync()
    {
        return await RequestAsync<string>("readAuthoringPrompt", null);
    }

    private async Task ReadStdoutAsync(Process process)
    {
        try
        {
            while (await process.StandardOutput.ReadLineAsync() is { } line)
            {
                using var document = JsonDocument.Parse(line);
                var root = document.RootElement;
                if (root.TryGetProperty("event", out var eventName)
                    && eventName.GetString() == "state"
                    && root.TryGetProperty("data", out var data))
                {
                    var snapshot = data.Deserialize<BackendAppSnapshot>(_json);
                    if (snapshot is not null)
                    {
                        SnapshotChanged?.Invoke(snapshot);
                    }
                    continue;
                }
                if (!root.TryGetProperty("id", out var idElement))
                {
                    continue;
                }
                var id = idElement.GetInt64();
                if (!_pending.TryGetValue(id, out var completion))
                {
                    continue;
                }
                if (root.TryGetProperty("error", out var error))
                {
                    var message = error.TryGetProperty("message", out var value)
                        ? value.GetString()
                        : PresentationFallback.Text(PresentationTextKey.AppBackendRequestFailed);
                    completion.TrySetException(new InvalidOperationException(message));
                }
                else if (root.TryGetProperty("result", out var result))
                {
                    completion.TrySetResult(result.Clone());
                }
                else
                {
                    completion.TrySetException(new InvalidOperationException(
                        PresentationFallback.Text(PresentationTextKey.AppBackendMalformed)));
                }
            }
        }
        catch (Exception exception)
        {
            App.Log($"Backend stdout failed: {exception}");
            FailPending(exception);
        }
    }

    private static async Task ReadStderrAsync(Process process)
    {
        while (await process.StandardError.ReadLineAsync() is { } line)
        {
            Debug.WriteLine($"[go-backend] {line}");
        }
    }

    private void HandleExit()
    {
        if (_stopping)
        {
            return;
        }
        var code = _process?.ExitCode ?? -1;
        var exception = new InvalidOperationException(PresentationFallback.Text(
            PresentationTextKey.AppBackendTerminated,
            ("status", code.ToString())));
        FailPending(exception);
        BackendFailed?.Invoke(exception.Message);
    }

    private void FailPending(Exception exception)
    {
        foreach (var completion in _pending.Values)
        {
            completion.TrySetException(exception);
        }
    }

    public async ValueTask DisposeAsync()
    {
        var process = _process;
        if (process is null)
        {
            return;
        }
        _stopping = true;
        try
        {
            if (!process.HasExited)
            {
                await RequestAsync<JsonElement>("shutdown", null).WaitAsync(TimeSpan.FromSeconds(2));
            }
        }
        catch
        {
            // The sidecar is private to this frontend and can be terminated on close.
        }
        _process = null;
        if (!process.HasExited)
        {
            process.Kill(entireProcessTree: true);
        }
        if (_stdoutTask is not null)
        {
            await _stdoutTask.ConfigureAwait(false);
        }
        if (_stderrTask is not null)
        {
            await _stderrTask.ConfigureAwait(false);
        }
        process.Dispose();
        _writeLock.Dispose();
    }
}
