import { render } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";
import QRCodeStyling from "qr-code-styling";
import "./style.css";

function themeColor(varName: string): string {
	return getComputedStyle(document.documentElement)
		.getPropertyValue(varName)
		.trim();
}

function useQRCode(data: string, visible: boolean) {
	const containerRef = useRef<HTMLDivElement>(null);
	const [qrCode] = useState<QRCodeStyling>(() => {
		return new QRCodeStyling({
			dotsOptions: {
				type: "extra-rounded",
				color: themeColor("color-primary-content"),
			},
			backgroundOptions: {
				color: themeColor("color-accent"),
			},
		});
	});

	useEffect(() => {
		const container = containerRef.current;
		if (!visible || !container) return;

		qrCode.append(container);
		qrCode.update({ data });

		const resizeToContainer = () => {
			const width = Math.floor(container.clientWidth);
			if (width > 0) {
				qrCode.update({ width, height: width });
			}
		};
		resizeToContainer();

		const observer = new ResizeObserver(resizeToContainer);
		observer.observe(container);
		return () => observer.disconnect();
	}, [qrCode, data, visible]);

	return containerRef;
}

function App() {
	return (
		<>
			<header>
				<div class="navbar bg-base-100 shadow-sm">
					<div class="flex-1">
						<a class="btn btn-ghost text-xl" href="/">
							InSecure GATEway
						</a>
					</div>
					<div class="flex-none">
						<form method="post" action="/signout">
							<button class="btn" type="submit">
								注销
							</button>
						</form>
					</div>
				</div>
			</header>
			<main class="flex flex-col flex-1 justify-center items-center">
				<Main />
			</main>
		</>
	);
}

function Main() {
	const [options, setOptions] = useState({
		url: "",
		exp: "3600",
	});
	const [result, setResult] = useState("");
	const [pending, setPending] = useState(false);

	const payload = options.url + options.exp.toString();
	const containerRef = useQRCode(payload, !!result);

	const onSubmit = async (e: Event) => {
		e.preventDefault();
		setPending(true);

		try {
			const form = new FormData();
			form.set("url", options.url);
			form.set("exp", options.exp);
			const resp = await fetch("/api/sign-url", {
				method: "POST",
				body: form,
			});
			setResult((await resp.json()).result.url);
		} catch (exc) {
			console.error(exc);
		} finally {
			setPending(false);
		}
	};

	const reset = async () => {
		setResult("");
		setOptions({
			...options,
			url: "",
		});
	};

	const copy = async () => {
		navigator.clipboard.writeText(result);
	};

	return result ? (
		<div>
			<div class="join w-full max-w-xs rounded-field focus-within:ring-2 focus-within:ring-base-content">
				<input
					id="result"
					type="text"
					class="input join-item grow outline-none"
					readOnly
					value={result}
				/>
				<button type="button" class="btn btn-square join-item" onClick={copy}>
					<svg
						class="size-[1.2em]"
						viewBox="0 0 24 24"
						fill="none"
						xmlns="http://www.w3.org/2000/svg"
					>
						<title>copy</title>
						<path
							d="M16 4C16.93 4 17.395 4 17.7765 4.10222C18.8117 4.37962 19.6204 5.18827 19.8978 6.22354C20 6.60504 20 7.07003 20 8V17.2C20 18.8802 20 19.7202 19.673 20.362C19.3854 20.9265 18.9265 21.3854 18.362 21.673C17.7202 22 16.8802 22 15.2 22H8.8C7.11984 22 6.27976 22 5.63803 21.673C5.07354 21.3854 4.6146 20.9265 4.32698 20.362C4 19.7202 4 18.8802 4 17.2V8C4 7.07003 4 6.60504 4.10222 6.22354C4.37962 5.18827 5.18827 4.37962 6.22354 4.10222C6.60504 4 7.07003 4 8 4M12 17V11M9 14H15M9.6 6H14.4C14.9601 6 15.2401 6 15.454 5.89101C15.6422 5.79513 15.7951 5.64215 15.891 5.45399C16 5.24008 16 4.96005 16 4.4V3.6C16 3.03995 16 2.75992 15.891 2.54601C15.7951 2.35785 15.6422 2.20487 15.454 2.10899C15.2401 2 14.9601 2 14.4 2H9.6C9.03995 2 8.75992 2 8.54601 2.10899C8.35785 2.20487 8.20487 2.35785 8.10899 2.54601C8 2.75992 8 3.03995 8 3.6V4.4C8 4.96005 8 5.24008 8.10899 5.45399C8.20487 5.64215 8.35785 5.79513 8.54601 5.89101C8.75992 6 9.03995 6 9.6 6Z"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						/>
					</svg>
				</button>
			</div>
			<div class="my-4 w-auto max-w-xs" ref={containerRef}></div>
			<button type="submit" class="btn w-xs" onClick={reset}>
				重新签名
			</button>
		</div>
	) : (
		<form onSubmit={onSubmit}>
			<fieldset class="fieldset" disabled={pending}>
				<label class="input">
					<svg
						class="size-[1.2em]"
						viewBox="0 0 24 24"
						fill="none"
						xmlns="http://www.w3.org/2000/svg"
					>
						<title>link</title>
						<path
							d="M21 18L19.9999 19.094C19.4695 19.6741 18.7502 20 18.0002 20C17.2501 20 16.5308 19.6741 16.0004 19.094C15.4693 18.5151 14.75 18.1901 14.0002 18.1901C13.2504 18.1901 12.5312 18.5151 12 19.094M3.00003 20H4.67457C5.16376 20 5.40835 20 5.63852 19.9447C5.84259 19.8957 6.03768 19.8149 6.21663 19.7053C6.41846 19.5816 6.59141 19.4086 6.93732 19.0627L19.5001 6.49998C20.3285 5.67156 20.3285 4.32841 19.5001 3.49998C18.6716 2.67156 17.3285 2.67156 16.5001 3.49998L3.93729 16.0627C3.59139 16.4086 3.41843 16.5816 3.29475 16.7834C3.18509 16.9624 3.10428 17.1574 3.05529 17.3615C3.00003 17.5917 3.00003 17.8363 3.00003 18.3255V20Z"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						/>
					</svg>
					<input
						onInput={(e) =>
							setOptions((prev) => ({
								...prev,
								url: e.currentTarget.value,
							}))
						}
						required
						type="url"
						class="grow"
						placeholder="https://"
					/>
				</label>
				<select
					onChange={(e) =>
						setOptions((prev) => ({
							...prev,
							exp: e.currentTarget.value,
						}))
					}
					class="select"
				>
					{[1, 3, 12, 24].map((h) => (
						<option
							value={h * 3600}
							selected={options.exp === (h * 3600).toString()}
						>
							{" "}
							{h} 小时{" "}
						</option>
					))}
				</select>
				<button type="submit" class="btn">
					签名
				</button>
			</fieldset>
		</form>
	);
}

render(<App />, document.getElementById("app") as HTMLElement);
