// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightThemeNova from 'starlight-theme-nova';
import starlightLinksValidator from 'starlight-links-validator';

const ref = (cmd) => ({ label: cmd, slug: `reference/jk_${cmd.replaceAll(' ', '_')}` });

export default defineConfig({
	site: 'https://phuquocchamp.github.io',
	base: '/jk',
	trailingSlash: 'always',
	integrations: [
		starlight({
			title: 'jk',
			description: 'Agent-first Jenkins CLI and MCP server',
			logo: { src: './src/assets/icon.svg' },
			favicon: '/favicon.svg',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/phuquocchamp/jk' }],
			editLink: { baseUrl: 'https://github.com/phuquocchamp/jk/edit/main/docs/' },
			lastUpdated: true,
			customCss: ['./src/styles/custom.css'],
			plugins: [
				// Fails the build on broken internal links (replaces mkdocs --strict).
				starlightLinksValidator(),
				starlightThemeNova({
					nav: [
						{ label: 'Guide', href: '/jk/getting-started/' },
						{ label: 'CLI reference', href: '/jk/reference/jk/' },
						{ label: 'MCP', href: '/jk/mcp/' },
					],
				}),
			],
			sidebar: [
				{
					label: 'Guide',
					items: [
						'getting-started',
						'authentication',
						'configuration',
						'policy',
						'tools',
						'mcp',
						'errors',
					],
				},
				{
					label: 'CLI reference',
					items: [
						{ label: 'jk', slug: 'reference/jk' },
						...['auth status', 'auth login', 'auth logout', 'job list', 'job get', 'build get',
							'build stages', 'build trigger', 'build replay', 'build stop', 'log tail',
							'log search', 'test failed', 'queue get', 'mcp', 'version', 'completion'].map(ref),
					],
				},
				{ label: 'Project', items: ['release'] },
			],
		}),
	],
});
