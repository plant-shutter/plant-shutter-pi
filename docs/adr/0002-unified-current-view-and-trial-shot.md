# Use one current view for preview, project photos, and tuning

**Status:** accepted

The user-facing workbench has one current view instead of exposing separate preview and capture modes. With no active shooting project it starts the live H.264 preview automatically; while a project is shooting it shows that project's latest JPEG and polls for updates. Creating another project is blocked until the active project is explicitly paused or ended. During project setup, a manual “试拍” temporarily captures one full-resolution JPEG, returns it directly to the browser, replaces the mobile preview until the user returns to live view, and is never written into a project or retained on the device.

The camera's mutually exclusive Preview and Capture states remain an internal device concern. This boundary keeps the user's mental model focused on the current view and the project lifecycle while preserving the existing hardware state machine.
