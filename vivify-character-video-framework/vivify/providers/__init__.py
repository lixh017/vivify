"""vivify.providers — adapters for AI asset generation vendors.

Each adapter is a `ProviderAdapter` subclass that knows how to call one
vendor (火山方舟 Ark, 海螺 MiniMax, ...) for one or more asset types
(video, image, tts, bgm).

The orchestrator (`vivify.asset_orchestrator`) is the only thing that
should call these directly — other code goes through the orchestrator.
"""
